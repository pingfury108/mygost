//go:build !windows

package proc

import "syscall"

// procAttr 让子进程拥有独立进程组，终止时只作用于 gost 自身。
func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
