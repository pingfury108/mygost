//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"strings"
)

// processCmdline 通过 CIM 取命令行。
func processCmdline(pid int) (string, error) {
	script := fmt.Sprintf(`(Get-CimInstance Win32_Process -Filter "ProcessId=%d").CommandLine`, pid)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
