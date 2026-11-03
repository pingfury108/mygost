//go:build !(darwin && amd64) && !(darwin && arm64) && !(windows && amd64)

package splitbin

// 该平台没有预编译插件：Plugin 返回 nil。
var platformPlugin []byte
