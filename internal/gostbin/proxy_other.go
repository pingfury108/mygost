//go:build !windows

package gostbin

// systemProxyPlatform 非 Windows 无系统级代理可读（环境变量已覆盖）。
func systemProxyPlatform() string { return "" }
