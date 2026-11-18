//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ─── Windows 进程操作：纯 Go 系统调用，不再 fork PowerShell ──────────
//
// 之前的实现每项操作都 spawn 一个 powershell（Get-CimInstance /
// tasklist / taskkill），冷启动 0.5~2 秒，且探测循环里高频调用，
// 会同时弹出一堆控制台窗口、吃满一个核。
//
// 现在全部走 windows API：
//
//	进程枚举+命令行  CreateToolhelp32Snapshot + Module32First
//	存活检测        OpenProcess + GetExitCodeProcess
//	终止            OpenProcess + TerminateProcess
//
// 快照本身带缓存：探测循环每 2 秒跑一次，短时间内的多次查询
// 复用同一份快照即可。

const (
	processQueryLimitedInformation = 0x1000
	processTerminate               = 0x0001
	stillActive                    = 259 // WAIT_TIMEOUT
)

// procEntry 是快照里的一条进程记录。
type procEntry struct {
	pid   int
	ppid  int
	exe   string
	image string // 可执行文件完整路径（懒加载，见 path()）
}

// snapshot 缓存一次枚举的结果，有效期 snapshotTTL。
// 并发探测（后台探测 + 用户操作）共享同一份，避免重复枚举。
var (
	snapMu      sync.Mutex
	snapshot    map[int]procEntry
	snapshotAt  time.Time
	snapshotTTL = 1 * time.Second
)

// listProcesses 枚举全部进程：pid → 命令行。
//
// Toolhelp32 只给 exe 名不给命令行；取命令行的常规做法是对每个 PID
// OpenProcess+QueryFullProcessImageName，仍拿不到完整命令行。
// 完整命令行要走 NtQueryInformationProcess 读 PEB，太重。
//
// 折衷：枚举一遍拿到 pid+exe；命令行按需用「父进程继承」技巧——
// 我们只关心 gost.exe 是否带特定 -C 参数，而 gost 的命令行在
// toolhelp 的 exe 名上是 "gost.exe"。真正需要命令行匹配的只有
// FindByConfig，它用 exe 名 + 我们写入的 PID 文件交叉验证就足够了。
func listProcesses() (map[int]procEntry, error) {
	snapMu.Lock()
	defer snapMu.Unlock()
	if snapshot != nil && time.Since(snapshotAt) < snapshotTTL {
		return snapshot, nil
	}

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	out := make(map[int]procEntry, 256)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	err = windows.Process32First(snap, &pe)
	for err == nil {
		exe := windows.UTF16ToString(pe.ExeFile[:])
		out[int(pe.ProcessID)] = procEntry{
			pid:  int(pe.ProcessID),
			ppid: int(pe.ParentProcessID),
			exe:  exe,
		}
		err = windows.Process32Next(snap, &pe)
	}
	snapshot = out
	snapshotAt = time.Now()
	return snapshot, nil
}

// FindByConfig 返回 exe 名为 gost 且 PID 未被其他记录认领的进程。
//
// Windows 拿不到可靠的其他进程命令行（需要读 PEB），所以这里以
// exe 名过滤；PID 文件（含精确 config）仍然是最权威的匹配方式，
// 这个枚举只作补充（接管残留/统计孤儿）。
// FindByConfig 返回 exe 名为 gost 的进程。
//
// exeDirs 非空时进一步限定"可执行文件位于这些目录下"——Windows 拿不到
// 其他进程的完整命令行，若只按 exe 名匹配，会把用户自己启动的 gost
// （甚至别的软件的 gost）也当成我们的残留进程，进而误杀。
func FindByConfig(configPath string, exeDirs ...string) []int {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}
	procs, err := listProcesses()
	if err != nil {
		return nil
	}
	var out []int
	for pid, e := range procs {
		if !isGostExe(e.exe) {
			continue
		}
		if len(exeDirs) == 0 {
			out = append(out, pid)
			continue
		}
		if underAnyDir(e.path(pid), exeDirs) {
			out = append(out, pid)
		}
	}
	return out
}

// path 返回进程可执行文件路径（按需查询并缓存）。
func (e *procEntry) path(pid int) string {
	if e.image != "" {
		return e.image
	}
	e.image = processImagePath(pid)
	return e.image
}

// underAnyDir 报告路径是否位于任一目录下（不区分大小写）。
func underAnyDir(p string, dirs []string) bool {
	if p == "" {
		return false
	}
	lp := strings.ToLower(filepath.Clean(p))
	for _, d := range dirs {
		if d == "" {
			continue
		}
		ld := strings.ToLower(filepath.Clean(d))
		if lp == ld || strings.HasPrefix(lp, ld+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// processImagePath 取进程可执行文件的完整路径。
//
// 与命令行不同，路径可以直接通过 OpenProcess +
// QueryFullProcessImageName 拿到，无需读 PEB。
func processImagePath(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// isGostExe 报告 exe 名是否是我们关心的 gost（不区分大小写）。
func isGostExe(exe string) bool {
	exe = strings.ToLower(strings.TrimSuffix(exe, ".exe"))
	return exe == "gost"
}

// PortOwner 返回监听端口的进程 pid。
//
// netstat 是普通控制台程序（毫秒级、无窗口闪烁），且只在端口冲突
// 诊断时调用（低频）。高频的探测已改走 PID 文件/探活，不走这里。
func PortOwner(port int) (pid int, name string) {
	if port <= 0 {
		return 0, ""
	}
	// 隐藏窗口：避免端口冲突诊断时闪一个黑框
	out, err := HideWindow(exec.Command("netstat", "-ano", "-p", "tcp")).Output()
	if err != nil {
		return 0, ""
	}
	suffix := fmt.Sprintf(":%d", port)
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || !strings.HasSuffix(f[1], suffix) || !strings.EqualFold(f[3], "LISTENING") {
			continue
		}
		if p, e := strconv.Atoi(f[4]); e == nil && p > 0 {
			return p, ""
		}
	}
	return 0, ""
}

// ProcessAlive 报告 pid 是否存活。
//
// OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION) + GetExitCodeProcess：
// 退出码仍是 STILL_ACTIVE 即存活。微秒级，无子进程。
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// 系统进程（0/4）视为存活
	if pid == 0 || pid == 4 {
		return true
	}
	h, err := windows.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false // 打不开 = 不存在或无权限
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// Terminate 强制结束进程及其子进程树。
//
// taskkill /T 的纯 Go 等价：枚举快照找全部后代，逐个 TerminateProcess。
// 强杀前没有优雅信号可用（gost 不处理 CTRL_BREAK），行为与原实现一致。
func Terminate(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("无效 pid")
	}
	// PID 可能被系统复用：确认目标确实是 gost 再动手
	if exe := strings.ToLower(filepath.Base(processImagePath(pid))); exe != "" &&
		exe != "gost.exe" {
		return fmt.Errorf("pid %d 不是 gost（%s），已跳过", pid, exe)
	}
	h, err := windows.OpenProcess(processTerminate|processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		if err == windows.ERROR_ACCESS_DENIED {
			return fmt.Errorf("无权限结束 pid %d", pid)
		}
		return fmt.Errorf("打开进程 %d 失败: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	// 先杀后代（快照缓存 1 秒内足够新鲜）
	if procs, err := listProcesses(); err == nil {
		for child, e := range procs {
			if e.ppid == pid {
				_ = terminateTree(procs, child, 0)
			}
		}
	}

	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("结束进程 %d 失败: %w", pid, err)
	}
	// 等它真正退出（最多 2 秒），避免调用方立刻又看到"存活"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err == nil && code != stillActive {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// terminateTree 递归结束后代进程。
func terminateTree(procs map[int]procEntry, pid int, depth int) error {
	if depth > 16 {
		return nil // 防御环
	}
	for child, e := range procs {
		if e.ppid == pid && child != pid {
			_ = terminateTree(procs, child, depth+1)
		}
	}
	h, err := windows.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

// processCmdline 返回进程的命令行。
//
// Toolhelp 拿不到完整命令行（在 PEB 里，需 NtQueryInformationProcess）。
// 之前的实现 fork PowerShell；现在只有 PID 文件校验这一个调用方，
// 而该校验的目标是"确认 PID 没被复用给别的程序"——用 exe 名判断
// 就足够了（PID 被复用时的 exe 几乎不可能还是 gost.exe）。
func processCmdline(pid int) (string, error) {
	procs, err := listProcesses()
	if err != nil {
		return "", err
	}
	if e, ok := procs[pid]; ok {
		return e.exe, nil
	}
	return "", fmt.Errorf("进程 %d 不存在", pid)
}

var _ = fmt.Sprintf // 保留 fmt 引用（错误信息使用）

// generateConsoleCtrlEvent 给进程组发 CTRL_BREAK_EVENT。
//
// gost 收到后走优雅关闭（监听器先关、连接排空），比 TerminateProcess
// 干净。目标进程必须以 CREATE_NEW_PROCESS_GROUP 创建才会只影响它。
func generateConsoleCtrlEvent(pid int) error {
	// FreeConsole + AttachConsole(pid) + GenerateConsoleCtrlEvent 是
	// 经典做法；Go 侧直接调 kernel32。
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	freeConsole := kernel32.NewProc("FreeConsole")
	attachConsole := kernel32.NewProc("AttachConsole")
	genCtrl := kernel32.NewProc("GenerateConsoleCtrlEvent")

	const ctrlBreakEvent = 1

	_, _, _ = freeConsole.Call()
	r1, _, callErr := attachConsole.Call(uintptr(pid))
	if r1 == 0 && callErr != nil && callErr.Error() != "The operation completed successfully." {
		// AttachConsole 可能报"已有控制台"，继续尝试发信号
		_ = callErr
	}
	defer func() {
		// 恢复我们自己的控制台（如果有）；失败无碍
		_, _, _ = attachConsole.Call(uintptr(0)) // 0 = 附到父控制台
	}()
	r2, _, _ := genCtrl.Call(ctrlBreakEvent, uintptr(pid))
	if r2 == 0 {
		return fmt.Errorf("GenerateConsoleCtrlEvent 失败")
	}
	return nil
}

// stopGostGraceful Windows 上的停止：先 CTRL_BREAK（优雅），
// 短暂等待后仍存活再强杀。返回是否已退出。
func stopGostGraceful(pid int, wait time.Duration) bool {
	if pid <= 0 {
		return false
	}
	if err := generateConsoleCtrlEvent(pid); err == nil {
		deadline := time.Now().Add(wait)
		for time.Now().Before(deadline) {
			if !ProcessAlive(pid) {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	// 优雅关闭失败/超时 → 强杀（含进程树）
	if err := Terminate(pid); err != nil {
		return !ProcessAlive(pid)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !ProcessAlive(pid) {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return !ProcessAlive(pid)
}

// matchesRecord 校验 pid 是否仍是记录里的那个进程。
//
// Windows 读不到其他进程的命令行，用可执行文件完整路径比对：
// 路径一致才可能是同一个进程（PID 复用时路径几乎必然不同）。
// 路径取不到时退回 exe 名判断，至少排除"被别的程序复用"。
func matchesRecord(rec Record) bool {
	exe := processImagePath(rec.PID)
	if exe == "" {
		return true // 拿不到信息时不否决，交给存活判断
	}
	if rec.Binary != "" {
		return strings.EqualFold(filepath.Clean(exe), filepath.Clean(rec.Binary))
	}
	return isGostExe(filepath.Base(exe))
}
