package splitbin

import (
	"runtime"
	"testing"
)

// 有预编译二进制的平台必须嵌入真实可执行文件。
func TestEmbeddedPlugin(t *testing.T) {
	supported := (runtime.GOOS == "darwin" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")) ||
		(runtime.GOOS == "windows" && runtime.GOARCH == "amd64")
	if !supported {
		t.Skipf("no prebuilt plugin for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	bin := Plugin()
	if len(bin) < 512*1024 {
		t.Fatalf("embedded plugin too small: %d bytes（先跑 ./tools/fetch-deps.sh）", len(bin))
	}
	var ok bool
	for _, magic := range [][]byte{{0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {'M', 'Z'}} {
		if len(bin) >= len(magic) && string(bin[:len(magic)]) == string(magic) {
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("not an executable (head %x)", bin[:4])
	}
	if Name() == "" {
		t.Error("Name empty")
	}
}

// 嵌入的是 gzip 流，Plugin() 必须解出真实可执行文件。
func TestPluginDecompresses(t *testing.T) {
	supported := (runtime.GOOS == "darwin" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")) ||
		(runtime.GOOS == "windows" && runtime.GOARCH == "amd64")
	if !supported {
		t.Skipf("no prebuilt plugin for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	raw := Plugin()
	if len(raw) < 512*1024 {
		t.Fatalf("解压后仅 %d 字节", len(raw))
	}
	// 必须是可执行文件头（Mach-O / PE）
	var ok bool
	for _, magic := range [][]byte{{0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {'M', 'Z'}} {
		if string(raw[:len(magic)]) == string(magic) {
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("解压结果不是可执行文件: % x", raw[:8])
	}
	// 压缩版应明显更小
	if c := len(Compressed()); c >= len(raw) {
		t.Errorf("压缩后 %d 不小于原始 %d", c, len(raw))
	} else {
		t.Logf("压缩 %d → 原始 %d（%.0f%%）", c, len(raw), 100*float64(c)/float64(len(raw)))
	}
}
