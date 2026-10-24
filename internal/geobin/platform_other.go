//go:build !(darwin && amd64) && !(darwin && arm64) && !(windows && amd64)

package geobin

// 该平台没有预编译插件：Plugin 返回 nil，geo.Manager 会退回手动安装。
var platformPlugin []byte
