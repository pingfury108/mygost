package gostbin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
