// Package splitbin 按平台嵌入 splitd 分流插件，使发布的 app 开箱即用。
//
// 二进制由 tools/fetch-deps.sh 生成（本仓库 cmd/splitd 交叉编译），不入版本库。
package splitbin

import (
	"bytes"
	"compress/gzip"
	"io"
	"runtime"
	"sync"
)

// Name 返回当前平台的插件文件名。
func Name() string {
	if runtime.GOOS == "windows" {
		return "splitd.exe"
	}
	return "splitd"
}

// Plugin 返回当前平台嵌入的插件二进制（解压后）；未嵌入时为 nil。
//
// 嵌入的是 gzip 压缩流（裸二进制约 10MB，压缩后 3.7MB）。解压只在
// 首次需要时发生——安装插件是一次性动作，之后从磁盘走。
func Plugin() []byte {
	once.Do(func() { unpacked = gunzip(platformPlugin) })
	return unpacked
}

// Compressed 返回嵌入的原始（gzip）字节，供体积统计与诊断。
func Compressed() []byte { return platformPlugin }

var (
	once     sync.Once
	unpacked []byte
)

// gunzip 解压；数据不是 gzip（如未嵌入）时返回 nil。
func gunzip(data []byte) []byte {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return nil // 空或非 gzip
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil
	}
	return out
}
