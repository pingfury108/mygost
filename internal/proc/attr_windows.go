//go:build windows

package proc

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideWindowAttr 是"不弹控制台窗口"的启动属性。
//
// Windows 上从 GUI 进程启动控制台程序时，系统会为它新建一个控制台窗口
// （黑框）。父进程若有控制台（从 cmd 启动）则子进程继承、不弹窗——
// 这就是"cmd 里启动没事、双击就闪黑框"的原因。
//
// CREATE_NEW_PROCESS_GROUP 让进程组独立，后续可发 CTRL_BREAK 优雅退出
// （否则该信号会打到整个控制台组）。
func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
}

// HideWindow 让 cmd 以无窗口方式启动（nil 安全）。
func HideWindow(cmd *exec.Cmd) *exec.Cmd {
	if cmd != nil {
		cmd.SysProcAttr = hideWindowAttr()
	}
	return cmd
}
