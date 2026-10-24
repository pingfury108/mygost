//go:build !(darwin && amd64) && !(darwin && arm64) && !(windows && amd64)

package gostbin

// 该平台没有内置 gost：Find 会退回下载 / PATH。
var embeddedArchive []byte
