package proc

import (
	"os/exec"
	"testing"
	"time"
)

// 接管：Attach 后 State=Running、Managed=false、Stop 能结束它。
func TestAttachAndStop(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn sleep: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	m := NewManager(nil)
	if err := m.Attach(cmd.Process.Pid, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if st, _, _ := m.State(); st != Running {
		t.Fatalf("state = %v, want Running", st)
	}
	if m.Managed() {
		t.Error("attached process should not be managed")
	}
	if m.Pid() != cmd.Process.Pid {
		t.Errorf("pid = %d, want %d", m.Pid(), cmd.Process.Pid)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if st, _, _ := m.State(); st == Running {
		t.Error("should be stopped after Stop")
	}
}

// 接管不存在的进程应当失败。
func TestAttachDeadPid(t *testing.T) {
	m := NewManager(nil)
	if err := m.Attach(999999, time.Time{}); err == nil {
		t.Error("attach to dead pid should fail")
	}
}
