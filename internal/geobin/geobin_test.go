package geobin

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
	if len(bin) < 1024*1024 {
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
		t.Fatalf("embedded plugin is not an executable (head %x)", bin[:4])
	}
	if Name() == "" {
		t.Error("Name should not be empty")
	}
}
