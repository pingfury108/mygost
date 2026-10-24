//go:build !windows

package proc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FindByConfig 扫描命令行包含 configPath 的 gost 进程。
// 用于识别没有 PID 文件的遗留进程（例如旧版本启动的）。
func FindByConfig(configPath string) []int {
	if strings.TrimSpace(configPath) == "" {
		return nil
	}
	out, err := exec.Command("pgrep", "-f", configPath).Output()
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
	if err := exec.Command("kill", "-TERM", strconv.Itoa(pid)).Run(); err != nil {
		return exec.Command("kill", "-9", strconv.Itoa(pid)).Run()
	}
	return nil
}
