// Package splitbin 按平台嵌入 splitd 分流插件，使发布的 app 开箱即用。
//
// 二进制由 tools/fetch-deps.sh 生成（本仓库 cmd/splitd 交叉编译），不入版本库。
package splitbin

import "runtime"

// Name 返回当前平台的插件文件名。
func Name() string {
	if runtime.GOOS == "windows" {
		return "splitd.exe"
	}
	return "splitd"
}

// Plugin 返回当前平台嵌入的插件二进制；未嵌入时为 nil。
func Plugin() []byte { return platformPlugin }
