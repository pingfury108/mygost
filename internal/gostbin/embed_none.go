//go:build !embedded

package gostbin

// 默认构建不嵌入 gost；embedded 构建由 CI 放置二进制到本包目录。
func ensureEmbedded(dir string) (string, bool) { return "", false }
