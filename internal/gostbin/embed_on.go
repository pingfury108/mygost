//go:build embedded

package gostbin

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
)

// gost 是构建时由脚本放置的对应平台 gost 二进制。
//
//go:embed gost
var gostBin []byte

// ensureEmbedded 首次运行时解出嵌入的二进制到 dir，返回其路径。
func ensureEmbedded(dir string) (string, bool) {
	if len(gostBin) == 0 {
		return "", false
	}
	final := filepath.Join(dir, binaryName())
	if st, err := os.Stat(final); err == nil && st.Size() == int64(len(gostBin)) {
		return final, true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, gostBin, 0o755); err != nil {
		return "", false
	}
	if runtime.GOOS != "windows" {
		os.Chmod(tmp, 0o755)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return "", false
	}
	return final, true
}
