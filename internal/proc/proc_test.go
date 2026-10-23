package proc

import (
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
