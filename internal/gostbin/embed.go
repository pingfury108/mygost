package gostbin

import (
	"os"
	"path/filepath"
	"runtime"
)

// embeddedBytes 由 platform_*.go 按平台各自 //go:embed 提供，
// 避免把别的平台的二进制打进包里；无内置的平台为 nil。

// embeddedDir 是嵌入副本的解压目录，与用户下载的副本分开存放，
// 这样"下载/更新"不会被嵌入副本覆盖，反之亦然。
func (l *Locator) embeddedDir() string {
	return filepath.Join(l.Dir, "embedded")
}

// ensureEmbedded 把嵌入的 gost 压缩包解到 embeddedDir（幂等：已解出即复用）。
func (l *Locator) ensureEmbedded() (string, bool) {
	if len(embeddedArchive) == 0 {
		return "", false
	}
	dir := l.embeddedDir()
	final := filepath.Join(dir, binaryName())
	if st, err := os.Stat(final); err == nil && st.Size() > 0 {
		return final, true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	bin, err := extractBinary(embeddedArchive, runtime.GOOS == "windows")
	if err != nil || len(bin) == 0 {
		return "", false
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", false
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", false
	}
	return final, true
}

// EmbeddedVersion 返回嵌入副本的版本描述（无嵌入时为空）。
func EmbeddedVersion() string {
	if len(embeddedArchive) == 0 {
		return ""
	}
	return DefaultVersion
}
