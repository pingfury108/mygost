package main

import (
	"os/exec"
	"testing"
	"time"

	"mygost/internal/proc"
	"mygost/internal/settings"
)

// Want 由 gost 状态与分流开关唯一决定。
func TestSupervisorWant(t *testing.T) {
	a := newTestApp(t)
	s := newSplitSupervisor(a)

	// 未连接 + 分流关 → 不该运行
	if s.Want() {
		t.Error("未连接且未开分流时不该期望运行")
	}

	// 分流开但 gost 未跑 → 仍不该运行
	a.st.Update(func(st *settings.Settings) { st.Split = true })
	if s.Want() {
		t.Error("gost 未运行时不该期望运行")
	}

	// gost 运行 + 分流开 → 应该运行
	// 用真实的 sleep 进程冒充 gost（Attach 会校验存活）
	cmd := exec.Command("sleep", "3")
	if err := cmd.Start(); err != nil {
		t.Skipf("无法启动占位进程: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if err := a.pm.Attach(cmd.Process.Pid, time.Now()); err != nil {
		t.Skipf("Attach 失败: %v", err)
	}
	if st, _, _ := a.pm.State(); st != proc.Running {
		t.Skip("无法伪造运行态")
	}
	if !s.Want() {
		t.Error("gost 运行且分流开启时应期望运行")
	}
}

// 失败退避按 1→2→4→8… 增长，并被上限截断。
func TestSupervisorBackoff(t *testing.T) {
	a := newTestApp(t)
	s := newSplitSupervisor(a)

	if s.backoff != splitBackoffMin {
		t.Fatalf("初始退避 = %v", s.backoff)
	}
	for i := 0; i < 10; i++ {
		s.noteErr(errTest)
	}
	if s.backoff != splitBackoffMax {
		t.Errorf("退避应被上限截断到 %v，实际 %v", splitBackoffMax, s.backoff)
	}
	// 成功后复位
	s.noteOK()
	if s.backoff != splitBackoffMin {
		t.Errorf("成功后应复位到 %v，实际 %v", splitBackoffMin, s.backoff)
	}
	if s.Err() != "" {
		t.Errorf("成功后错误应清空，实际 %q", s.Err())
	}
}

// 退避期内不允许重试，避免每轮都跑一次 15 秒的就绪等待。
func TestSupervisorRetryGate(t *testing.T) {
	a := newTestApp(t)
	s := newSplitSupervisor(a)
	s.noteErr(errTest) // 记录一次失败 → 进入退避
	if s.allowRetry() {
		t.Error("刚失败后不应立即重试")
	}
	s.mu.Lock()
	s.lastTry = time.Now().Add(-2 * time.Second) // 时间往后拨
	s.mu.Unlock()
	if !s.allowRetry() {
		t.Error("退避过后应允许重试")
	}
}

type testErr struct{}

func (testErr) Error() string { return "test failure" }

var errTest = testErr{}
