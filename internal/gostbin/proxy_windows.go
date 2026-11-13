//go:build windows

package gostbin

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxyPlatform 读 Windows 注册表里的用户代理设置。
//
// 路径：HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings
// ProxyEnable=1 且 ProxyServer 非空时返回 "http://<server>"。
func systemProxyPlatform() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return ""
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil || strings.TrimSpace(server) == "" {
		return ""
	}
	server = strings.TrimSpace(server)
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}
	return server
}
