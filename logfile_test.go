package main

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no console") }

// Windows GUI 没有控制台时 stderr 写失败，文件日志仍必须写入。
func TestLogOutputWritesFileWhenStderrFails(t *testing.T) {
	var file bytes.Buffer
	w := logOutput(&file, failingWriter{})
	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := file.String(); got != "hello\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestLogOutputWithoutStderr(t *testing.T) {
	var file bytes.Buffer
	w := logOutput(&file, nil)
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if file.String() != "x" {
		t.Fatalf("file = %q", file.String())
	}
}

// setupLogging 必须真正把日志写到 logs/mygost.log。
func TestSetupLoggingWritesFile(t *testing.T) {
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	}()

	dir := t.TempDir()
	path, closeFn := setupLogging(dir)
	if path == "" {
		t.Fatal("setupLogging 未返回日志路径")
	}
	const marker = "mygost-setup-logging-test-marker"
	log.Print(marker)
	closeFn()

	data, err := os.ReadFile(filepath.Join(dir, "logs", "mygost.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), marker) {
		t.Fatalf("日志文件缺少测试标记: %q", data)
	}
}
