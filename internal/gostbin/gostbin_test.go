package gostbin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testOS() string { return runtime.GOOS }

// buildTestTarGz 构造一个含 gost 文件的内存 tar.gz。
func buildTestTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte("FAKEGOST")
	if err := tw.WriteHeader(&tar.Header{Name: "gost_3.3.1_linux_amd64/gost", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestDownloadURLDefault(t *testing.T) {
	u := DownloadURL("")
	if !strings.Contains(u, "github.com/go-gost/gost/releases") {
		t.Errorf("unexpected url %q", u)
	}
	if !strings.Contains(u, "gost_3.3.1_"+testOS()+"_") {
		t.Errorf("url should contain version/os/arch: %q", u)
	}
}

func TestDownloadURLMirror(t *testing.T) {
	u := DownloadURL("https://mirror.example.com/gost/v{version}/{bin}")
	if !strings.HasPrefix(u, "https://mirror.example.com/gost/v3.3.1/") {
		t.Errorf("mirror template not expanded: %q", u)
	}
}

func TestChecksumOK(t *testing.T) {
	sums := "abc123  gost_3.3.1_linux_amd64.tar.gz\ndef456  gost_3.3.1_darwin_amd64.tar.gz\n"
	if !checksumOK(sums, "gost_3.3.1_linux_amd64.tar.gz", "abc123") {
		t.Error("matching checksum should pass")
	}
	if checksumOK(sums, "gost_3.3.1_linux_amd64.tar.gz", "wrong") {
		t.Error("wrong checksum should fail")
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	data := buildTestTarGz(t)
	got, err := extractBinary(data, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "FAKEGOST" {
		t.Errorf("got %q", got)
	}
}

func TestProgressReader(t *testing.T) {
	pr := &progressReader{r: strings.NewReader("hello"), total: 5, on: func(done, total int64) {}}
	buf := make([]byte, 16)
	n, err := pr.Read(buf)
	if err != nil || n != 5 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if pr.done != 5 {
		t.Errorf("done=%d", pr.done)
	}
	_ = time.Now()
}

// 优先级：已下载副本必须盖过内置版本，否则「下载/更新」永远不生效。
func TestFindPrefersDownloadedOverEmbedded(t *testing.T) {
	dir := t.TempDir()
	l := &Locator{Dir: dir}

	// 先只有内置：应返回 embedded（并解压到 embedded 子目录）
	p, src, err := l.Find("")
	if err != nil {
		t.Skipf("no embedded gost on this platform: %v", err)
	}
	if src != "embedded" {
		t.Fatalf("source = %q, want embedded", src)
	}
	if !strings.Contains(p, "embedded") {
		t.Errorf("embedded copy should live under embedded/: %q", p)
	}

	// 再放一个"下载副本"：应优先生效
	dl := filepath.Join(dir, binaryName())
	if err := os.WriteFile(dl, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	p2, src2, err := l.Find("")
	if err != nil {
		t.Fatal(err)
	}
	if src2 != "downloaded" || p2 != dl {
		t.Errorf("downloaded should win: source=%q path=%q", src2, p2)
	}

	// 手动路径优先级最高
	manual := filepath.Join(dir, "my-gost")
	if err := os.WriteFile(manual, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, src3, _ := l.Find(manual); src3 != "manual" {
		t.Errorf("manual should win, got %q", src3)
	}
}

// 内置二进制必须是当前平台的可执行文件。
func TestEmbeddedGostBinary(t *testing.T) {
	if len(embeddedArchive) == 0 {
		t.Skipf("no embedded gost for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if len(embeddedArchive) < 5*1024*1024 {
		t.Fatalf("embedded gost archive too small: %d bytes（先跑 ./tools/fetch-deps.sh）", len(embeddedArchive))
	}
	if EmbeddedVersion() == "" {
		t.Error("EmbeddedVersion should be set when embedded")
	}
}
