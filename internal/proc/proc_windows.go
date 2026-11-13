//go:build windows

package proc

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// procAttr 让 gost 拥有独立控制台与进程组：
//
//	CREATE_NEW_PROCESS_GROUP 让它能收到 CTRL_BREAK_EVENT（否则信号
//	会发给整个控制台组，误伤别的子进程）
//	CREATE_NO_WINDOW 避免 UI 弹出 gost 的控制台窗口
func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
}
