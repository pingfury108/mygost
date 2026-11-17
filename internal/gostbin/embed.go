package gostbin

import (
	"errors"
	"fmt"
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
	p, err := l.installEmbedded()
	if err != nil {
		// 记下原因：Windows 上常见的是杀软拦截写盘/改名，
		// 界面上只显示"未安装"会让用户无从排查
		l.lastErr = err
	}
	return p, err == nil
}

// installEmbedded 把嵌入的压缩包解到本地并返回可执行路径。
func (l *Locator) installEmbedded() (string, error) {
	if len(embeddedArchive) == 0 {
		return "", errNoEmbed
	}
	dir := l.embeddedDir()
	final := filepath.Join(dir, binaryName())
	if st, err := os.Stat(final); err == nil && st.Size() > 0 {
		return final, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录 %s 失败: %w", dir, err)
	}
	bin, err := extractBinary(embeddedArchive, runtime.GOOS == "windows")
	if err != nil {
		return "", fmt.Errorf("解压内置 gost 失败: %w", err)
	}
	if len(bin) == 0 {
		return "", fmt.Errorf("解压内置 gost 得到空文件")
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return "", fmt.Errorf("写入 %s 失败（可能被安全软件拦截）: %w", tmp, err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("重命名到 %s 失败（可能被安全软件拦截）: %w", final, err)
	}
	return final, nil
}

// LastErr 返回最近一次安装内置副本的失败原因（成功时为 nil）。
func (l *Locator) LastErr() error {
	if l == nil {
		return nil
	}
	return l.lastErr
}

var errNoEmbed = errors.New("当前平台未内置 gost")

// EmbeddedVersion 返回嵌入副本的版本描述（无嵌入时为空）。
func EmbeddedVersion() string {
	if len(embeddedArchive) == 0 {
		return ""
	}
	return DefaultVersion
}

// EmbeddedSize 返回当前平台嵌入的 gost 压缩包字节数（体积分析用）。
func EmbeddedSize() int { return len(embeddedArchive) }
