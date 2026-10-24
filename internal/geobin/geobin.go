// Package geobin 按平台嵌入 gost-geo-plugin 二进制，使发布的 app 开箱即用，
// 用户无需自行安装插件。
//
// 二进制由 tools/fetch-deps.sh 生成（交叉编译各平台），不进版本库：
//
//	./tools/fetch-deps.sh    # 生成 internal/geobin/gost-geo-plugin_<os>_<arch>
//
// 未生成时可加 -tags nogeoembed 构建（插件退化为需手动安装）。
package geobin

import "runtime"

// Name 返回当前平台的插件文件名。
func Name() string {
	if runtime.GOOS == "windows" {
		return "gost-geo-plugin.exe"
	}
	return "gost-geo-plugin"
}

// Plugin 返回当前平台嵌入的插件二进制；未嵌入时为 nil。
func Plugin() []byte { return platformPlugin }
