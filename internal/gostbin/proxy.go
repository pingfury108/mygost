package gostbin

import (
	"os"
	"strings"
)

// systemProxy 返回系统代理设置。
//
// 读取顺序（第一个非空生效）：
//  1. HTTPS_PROXY / https_proxy 环境变量（开发者常用）
//  2. Windows 注册表 Internet Settings（多数 Windows 用户靠它上网）
//  3. 空表示直连
//
// macOS/Linux 的系统级代理没有统一入口（macOS 在 scutil，
// Linux 在桌面环境），保持读环境变量即可。
func systemProxy() string {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return systemProxyPlatform()
}
