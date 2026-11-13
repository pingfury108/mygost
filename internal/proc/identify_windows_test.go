//go:build windows

package proc

import (
	"os"
	"testing"
	"time"
)

// ProcessAlive 对当前进程必须为 true。
func TestProcessAliveSelf(t *testing.T) {
	if !ProcessAlive(os.Getpid()) {
		t.Error("自身进程应判定存活")
	}
}

// 已退出的 pid 判定为 false，且不能卡住（历史上 powershell 要 1 秒）。
func TestProcessAliveFastOnDeadPid(t *testing.T) {
	start := time.Now()
	alive := ProcessAlive(999999)
	elapsed := time.Since(start)
	if alive {
		t.Error("不存在的 pid 应为 false")
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("探活耗时 %v，应 < 50ms（纯 API 调用）", elapsed)
	}
}

// 进程枚举必须在毫秒级完成且带缓存。
func TestListProcessesFast(t *testing.T) {
	start := time.Now()
	procs, err := listProcesses()
	first := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) == 0 {
		t.Fatal("枚举结果为空")
	}
	if first > 500*time.Millisecond {
		t.Errorf("首次枚举 %v，应 < 500ms", first)
	}

	start = time.Now()
	if _, err := listProcesses(); err != nil {
		t.Fatal(err)
	}
	if second := time.Since(start); second > 5*time.Millisecond {
		t.Errorf("缓存命中应 < 5ms，实际 %v", second)
	}
}

// 自进程应出现在枚举结果里。
func TestListProcessesContainsSelf(t *testing.T) {
	procs, err := listProcesses()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := procs[os.Getpid()]; !ok {
		t.Error("枚举结果应包含当前进程")
	}
}

// FindByConfig 不因无匹配而卡顿（历史上会 spawn powershell）。
func TestFindByConfigFast(t *testing.T) {
	start := time.Now()
	_ = FindByConfig(`C:\nonexistent\profiles\abc.yml`)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("FindByConfig 耗时 %v，应 < 500ms", elapsed)
	}
}
