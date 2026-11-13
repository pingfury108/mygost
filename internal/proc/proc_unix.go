//go:build !windows

package proc

import (
	"syscall"
	"time"
)

// procAttr 让子进程拥有独立进程组，终止时只作用于 gost 自身。
func procAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// stopGostGraceful unix 的优雅停止桩：SIGTERM 路径已在 stopManaged
// 里处理（unix 上 Process.Signal 就是优雅信号），这里恒返回已退出。
func stopGostGraceful(pid int, wait time.Duration) bool {
	_ = pid
	_ = wait
	return true
}
