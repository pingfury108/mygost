// Package proc 托管 gost 子进程：启动、停止、重启、日志环形缓冲。
package proc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// State 是 gost 进程状态。
type State int32

const (
	Stopped State = iota // 未运行（或已手动停止）
	Running              // 运行中
	Crashed              // 异常退出
)

func (s State) String() string {
	switch s {
	case Running:
		return "运行中"
	case Crashed:
		return "异常退出"
	default:
		return "已停止"
	}
}

// ErrAlreadyRunning 已在运行。
var ErrAlreadyRunning = errors.New("gost 已在运行")

// StartError 表示进程在启动窗口内就退出了（如端口被占用）。
type StartError struct {
	Reason string   // 进程退出原因
	Logs   []string // 最后几行输出，通常含 gost 的错误原因
}

func (e *StartError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "进程启动后立即退出"
}

// Detail 返回用于界面的可读说明（原因 + 最后一行有效日志）。
func (e *StartError) Detail() string {
	msg := e.Error()
	for i := len(e.Logs) - 1; i >= 0; i-- {
		l := strings.TrimSpace(e.Logs[i])
		if l == "" || strings.Contains(l, "→ 启动:") {
			continue
		}
		if idx := strings.Index(l, "\"msg\":"); idx >= 0 {
			rest := l[idx+len("\"msg\":"):]
			rest = strings.TrimPrefix(strings.TrimSpace(rest), "\"")
			if end := strings.Index(rest, "\""); end > 0 {
				rest = rest[:end]
			}
			if rest != "" && rest != "listen tcp: bind: address already in use" {
				return msg + "：" + rest
			}
			if rest != "" {
				return msg
			}
		}
		return msg + "：" + l
	}
	return msg
}

// Manager 托管一个 gost 进程。notify 在状态或日志变化后被调用
// （约 150ms 合并节流），用于触发界面刷新；它运行在独立 goroutine。
type Manager struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	state  State
	manual bool
	errStr string
	start  time.Time

	exited chan struct{} // 每次 Start 新建，进程退出时关闭
	extPID int           // >0 表示接管的外部进程（非本会话 exec 启动）

	logMu   sync.Mutex
	logs    []string
	logNext int
	logFull bool

	notifyMu sync.Mutex
	pending  bool
	notify   func()
}

// NewManager 创建管理器；notify 为nil时不回调。
// 日志默认保留最近 5000 行。
func NewManager(notify func()) *Manager {
	return &Manager{notify: notify, logs: make([]string, 5000)}
}

// Start 以 bin -C config 启动 gost。
func (m *Manager) Start(bin, config string) error {
	m.mu.Lock()
	if m.state == Running {
		m.mu.Unlock()
		return ErrAlreadyRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "-C", config)
	// 让 gost 进程组独立于 UI，避免部分平台信号行为差异
	cmd.SysProcAttr = procAttr()
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		m.mu.Unlock()
		return err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		cancel()
		pw.Close()
		pr.Close()
		m.state = Crashed
		m.errStr = err.Error()
		m.mu.Unlock()
		m.notifySoon()
		return err
	}
	m.cmd = cmd
	m.cancel = cancel
	m.extPID = 0
	m.exited = make(chan struct{})
	m.state = Running
	m.manual = false
	m.errStr = ""
	m.start = time.Now()
	pw.Close() // 写端交给子进程，读端在 goroutine
	m.mu.Unlock()

	go m.pump(pr)
	go m.watch(cmd, cancel)
	m.pushLine(fmt.Sprintf("→ 启动: %s -C %s", bin, config))
	m.notifySoon()
	return nil
}

// WaitStable 等待 d 的启动确认窗口：
// 期间进程退出则返回 *StartError（例如 gost 绑定端口失败会立刻退出）。
func (m *Manager) WaitStable(d time.Duration) error {
	m.mu.Lock()
	ch := m.exited
	m.mu.Unlock()
	if ch == nil {
		return &StartError{Reason: "进程未启动"}
	}

	select {
	case <-ch: // 窗口内退出：多半是端口占用、配置错误等
		st, _, reason := m.State()
		if st == Running {
			return nil
		}
		return &StartError{Reason: reason, Logs: m.Lines(3)}
	case <-time.After(d):
		// 超时后必须复核：高负载下退出信号可能刚好在边界到达
		if st, _, reason := m.State(); st != Running {
			return &StartError{Reason: reason, Logs: m.Lines(3)}
		}
		return nil
	}
}

// Stop 手动停止并等待进程退出（最多 timeout，超时放弃等待）。
func (m *Manager) Stop() error {
	if err := m.stopAsync(); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, _, _ := m.State(); st != Running {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// StopAndWait 同 Stop，可自定义超时。
func (m *Manager) StopAndWait(timeout time.Duration) error {
	if err := m.stopAsync(); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, _, _ := m.State(); st != Running {
			return nil
		}
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// stopAsync 发起停止（不等待）。
func (m *Manager) stopAsync() error {
	m.mu.Lock()
	if m.state != Running {
		m.mu.Unlock()
		return nil
	}
	// 接管进程：没有 cmd，按 pid 结束
	if m.extPID > 0 {
		pid := m.extPID
		m.mu.Unlock()
		m.pushLine(fmt.Sprintf("→ 结束接管进程 pid=%d", pid))
		if err := Terminate(pid); err != nil {
			return err
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && ProcessAlive(pid) {
			time.Sleep(50 * time.Millisecond)
		}
		m.mu.Lock()
		if !ProcessAlive(pid) {
			m.extPID = 0
			m.state = Stopped
		}
		m.mu.Unlock()
		m.notifySoon()
		return nil
	}
	if m.cmd == nil {
		m.mu.Unlock()
		return nil
	}
	m.manual = true
	cmd := m.cmd
	m.mu.Unlock()
	m.pushLine("→ 收到停止请求")
	terminate(cmd)
	// watch goroutine 负责置为 Stopped
	return nil
}

// Restart 停止后重新启动（同一配置）。
func (m *Manager) Restart(bin, config string) error {
	if err := m.Stop(); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, _, _ := m.State()
		if st != Running {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return m.Start(bin, config)
}

// Attach 接管一个已存在的 gost 进程（例如上次会话残留）。
// 接管后 State 报 Running，Stop 仍可结束它；但读不到它的输出。
func (m *Manager) Attach(pid int, started time.Time) error {
	if pid <= 0 {
		return ErrNoProcess
	}
	if !ProcessAlive(pid) {
		return ErrNoProcess
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == Running {
		return ErrAlreadyRunning
	}
	m.extPID = pid
	m.state = Running
	m.manual = false
	m.errStr = ""
	if started.IsZero() {
		started = time.Now()
	}
	m.start = started
	m.pushLine(fmt.Sprintf("→ 接管已有进程 pid=%d（读不到它的输出）", pid))
	m.notifySoon()
	return nil
}

// Managed 报告当前进程是否由本会话启动（false = 接管而来）。
func (m *Manager) Managed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil
}

// ExternalPID 返回接管进程的 pid（0 表示没有接管）。
func (m *Manager) ExternalPID() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.extPID
}

// Pid 返回子进程 PID（未运行时为 0），便于诊断与测试。
func (m *Manager) Pid() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.extPID > 0 {
		return m.extPID
	}
	if m.cmd == nil || m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

// State 返回当前状态、启动时间与最后的错误信息。
func (m *Manager) State() (State, time.Time, string) {
	m.mu.Lock()
	ext := m.extPID
	st, started, reason := m.state, m.start, m.errStr
	m.mu.Unlock()

	// 接管进程可能在外部退出：惰性校验
	if ext > 0 && st == Running && !ProcessAlive(ext) {
		m.mu.Lock()
		if m.extPID == ext {
			m.extPID = 0
			m.state = Stopped
			m.pushLine("← 接管的进程已退出")
		}
		st = m.state
		m.mu.Unlock()
	}
	return st, started, reason
}

// Lines 返回最近 max 行日志（max<=0 返回全部保留的）。
func (m *Manager) Lines(max int) []string {
	m.logMu.Lock()
	defer m.logMu.Unlock()
	var out []string
	if m.logFull {
		out = append(out, m.logs[m.logNext:]...)
	}
	out = append(out, m.logs[:m.logNext]...)
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

// LogText 拼接全部日志。
func (m *Manager) LogText() string {
	return joinLines(m.Lines(0))
}

// ClearLogs 清空日志。
func (m *Manager) ClearLogs() {
	m.logMu.Lock()
	m.logNext = 0
	m.logFull = false
	m.logMu.Unlock()
	m.notifySoon()
}

func joinLines(lines []string) string {
	buf := make([]byte, 0, 4096)
	for i, l := range lines {
		if i > 0 {
			buf = append(buf, '\n')
		}
		buf = append(buf, l...)
	}
	return string(buf)
}

// pump 持续读取子进程输出。
func (m *Manager) pump(r *os.File) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		m.pushLine(sc.Text())
		m.notifySoon()
	}
}

// watch 等待进程退出并更新状态。
func (m *Manager) watch(cmd *exec.Cmd, cancel context.CancelFunc) {
	err := cmd.Wait()
	cancel()
	m.mu.Lock()
	manual := m.manual
	m.cmd = nil
	m.cancel = nil
	if manual {
		m.state = Stopped
	} else {
		m.state = Crashed
		if err != nil {
			m.errStr = err.Error()
		}
	}
	m.mu.Unlock()
	if ch := m.exited; ch != nil {
		close(ch)
	}
	m.pushLine(fmt.Sprintf("← 进程退出 (%s)", m.state))
	m.notifySoon()
}

func (m *Manager) pushLine(line string) {
	if line == "" {
		return
	}
	ts := time.Now().Format("15:04:05 ")
	m.logMu.Lock()
	m.logs[m.logNext] = ts + line
	m.logNext++
	if m.logNext >= len(m.logs) {
		m.logNext = 0
		m.logFull = true
	}
	m.logMu.Unlock()
}

// notifySoon 节流回调，避免高频日志刷爆主线程。
func (m *Manager) notifySoon() {
	if m.notify == nil {
		return
	}
	m.notifyMu.Lock()
	if m.pending {
		m.notifyMu.Unlock()
		return
	}
	m.pending = true
	m.notifyMu.Unlock()
	time.AfterFunc(150*time.Millisecond, func() {
		m.notifyMu.Lock()
		m.pending = false
		m.notifyMu.Unlock()
		m.notify()
	})
}

// terminate 平台相关的终止逻辑。
func terminate(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		cmd.Process.Kill()
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		cmd.Process.Kill()
		return
	}
	go func() {
		time.Sleep(2 * time.Second)
		// 仍未退出则强杀（Kill 已退出进程返回错误，忽略）
		cmd.Process.Kill()
	}()
}
