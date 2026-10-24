//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FindByConfig 在 Windows 上通过 CIM 查询命令行含 configPath 的进程。
// 查询失败时返回 nil（降级为仅依赖 PID 文件）。
func FindByConfig(configPath string) []int {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}
	script := fmt.Sprintf(
		`Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -and $_.CommandLine.Contains('%s') } | Select-Object -ExpandProperty ProcessId`,
		strings.ReplaceAll(configPath, "'", "''"))
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil
	}
	return parsePids(string(out), 0)
}

// PortOwner 通过 netstat 找监听端口的 pid（进程名留空）。
func PortOwner(port int) (pid int, name string) {
	if port <= 0 {
		return 0, ""
	}
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
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
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}

// Terminate 结束进程（Windows 无优雅信号，直接 taskkill）。
func Terminate(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("无效 pid")
	}
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
}
