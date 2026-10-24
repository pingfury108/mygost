//go:build !windows

package proc

import (
	"os/exec"
	"strconv"
)

// processCmdline 返回进程的完整命令行。
func processCmdline(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
