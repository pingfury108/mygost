//go:build !windows

package proc

import "os/exec"

// HideWindow 在非 Windows 平台无作用：没有控制台窗口这回事。
func HideWindow(cmd *exec.Cmd) *exec.Cmd { return cmd }
