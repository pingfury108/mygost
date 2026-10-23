//go:build windows

package proc

import "syscall"

func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
