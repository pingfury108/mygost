//go:build !windows

package proc

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// FindByConfig 扫描命令行包含 configPath 的 gost 进程。
// 用于识别没有 PID 文件的遗留进程（例如旧版本启动的）。
func FindByConfig(configPath string, _ ...string) []int {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}
	out, err := exec.Command("pgrep", "-f", regexp.QuoteMeta(configPath)).Output()
	if err != nil {
		return nil
	}
	return parsePids(string(out), 0)
}

// PortOwner 返回监听 addr 端口的进程 pid 与命令名。
func PortOwner(port int) (pid int, name string) {
	if port <= 0 {
		return 0, ""
	}
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpc").Output()
	if err != nil {
		return 0, ""
	}
	pids := parsePids(string(out), 0)
	if len(pids) == 0 {
		return 0, ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "c") {
			name = strings.TrimSpace(line[1:])
			break
		}
	}
	return pids[0], name
}

// ProcessAlive 报告 pid 是否存活。
// 僵尸进程（已退出未回收）视为已死：kill -0 对僵尸仍成功，会误导停止流程。
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	stat := strings.TrimSpace(string(out))
	if stat == "" || strings.HasPrefix(stat, "Z") {
		return false
	}
	return true
}

// Terminate 先 SIGTERM，超时后 SIGKILL。
func Terminate(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("无效 pid")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !ProcessAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ProcessAlive(pid) {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}

// processImagePath 返回进程可执行文件路径。
func processImagePath(pid int) string {
	out, err := exec.Command("ps", "-o", "comm=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// underAnyDir 报告路径是否位于任一目录下。
func underAnyDir(p string, dirs []string) bool {
	if p == "" {
		return false
	}
	cp := filepath.Clean(p)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		cd := filepath.Clean(d)
		if cp == cd || strings.HasPrefix(cp, cd+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// matchesRecord 校验 pid 是否仍是记录里的那个进程（unix：命令行含配置路径）。
func matchesRecord(rec Record) bool {
	if rec.Config == "" {
		return true
	}
	return cmdlineContains(rec.PID, rec.Config)
}
