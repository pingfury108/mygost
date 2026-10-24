package proc

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRingBuffer(t *testing.T) {
	m := NewManager(nil)
	for i := 0; i < 6000; i++ {
		m.pushLine("line")
	}
	lines := m.Lines(0)
	if len(lines) != 5000 {
		t.Fatalf("expected 5000 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[len(lines)-1], "line") {
		t.Errorf("last line wrong: %q", lines[len(lines)-1])
	}
}

func TestLinesMax(t *testing.T) {
	m := NewManager(nil)
	for i := 0; i < 100; i++ {
		m.pushLine("l")
	}
	if got := len(m.Lines(10)); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestClearLogs(t *testing.T) {
	m := NewManager(nil)
	m.pushLine("x")
	m.ClearLogs()
	if len(m.Lines(0)) != 0 {
		t.Fatal("should be empty")
	}
}

func TestStateDefaults(t *testing.T) {
	m := NewManager(nil)
	st, start, errStr := m.State()
	if st != Stopped || !start.IsZero() || errStr != "" {
		t.Fatalf("unexpected initial state: %v %v %q", st, start, errStr)
	}
}

func TestStopWhenNotRunning(t *testing.T) {
	m := NewManager(nil)
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	_ = time.Now
}

// 启动确认：进程拉起后立刻退出（如端口被占用）必须返回错误，
// 而不是让界面误报"已连接"。
func TestWaitStableDetectsEarlyExit(t *testing.T) {
	// 用 /usr/bin/true 模拟"进程拉起来了但立刻退出"（真实场景：端口占用时 gost fatal）。
	// 不用 shell 脚本：测试环境下 sh 有时会挂在读 stdin 上，造成假失败。
	tru := "/usr/bin/true"
	if _, err := os.Stat(tru); err != nil {
		t.Skip("no /usr/bin/true on this platform")
	}
	m := NewManager(nil)
	defer m.Stop()
	if err := m.Start(tru, "cfg.yml"); err != nil {
		t.Fatalf("spawn should succeed: %v", err)
	}
	err := m.WaitStable(2 * time.Second)
	if err == nil {
		st, _, reason := m.State()
		t.Fatalf("early exit must be reported as an error (state=%v reason=%q)", st, reason)
	}
	var se *StartError
	if !errors.As(err, &se) {
		t.Fatalf("want *StartError, got %T: %v", err, err)
	}
	if se.Detail() == "" {
		t.Error("detail should explain the failure")
	}
}

// 存活进程在确认窗口后应视为启动成功。
func TestWaitStableOK(t *testing.T) {
	dir := t.TempDir()
	bin := dir + "/fake-gost"
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(nil)
	if err := m.Start(bin, "cfg.yml"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer m.Stop()
	if err := m.WaitStable(300 * time.Millisecond); err != nil {
		t.Fatalf("running process should pass: %v", err)
	}
}
