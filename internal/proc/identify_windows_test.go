//go:build windows

package proc

import (
	"net"
	"os"
	"os/exec"
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

// 只在指定目录下的 gost.exe 才算我们的进程：避免误杀用户自己的 gost。
func TestFindByConfigScopedByDir(t *testing.T) {
	// 当前系统上不一定有 gost 进程，这里只验证"不同目录不匹配"的语义
	dirs := []string{`C:\definitely\not\matching`}
	got := FindByConfig(`C:\profiles`, dirs...)
	for _, pid := range got {
		exe := processImagePath(pid)
		if exe == "" {
			continue
		}
		if underAnyDir(exe, dirs) {
			t.Errorf("pid %d (%s) 不应匹配 %v", pid, exe, dirs)
		}
	}
}

// 路径归属判断：不区分大小写，且要求是目录前缀而非字符串前缀。
func TestUnderAnyDir(t *testing.T) {
	dirs := []string{`C:\Users\me\AppData\Roaming\mygost\bin`}
	cases := map[string]bool{
		`C:\Users\me\AppData\Roaming\mygost\bin\gost.exe`:          true,
		`c:\users\me\appdata\roaming\mygost\BIN\gost.exe`:          true,
		`C:\Users\me\AppData\Roaming\mygost\bin\embedded\gost.exe`: true,
		`C:\Users\me\AppData\Roaming\mygost\binary\gost.exe`:       false,
		`D:\下载\gost.exe`: false,
	}
	for p, want := range cases {
		if got := underAnyDir(p, dirs); got != want {
			t.Errorf("underAnyDir(%q) = %v, want %v", p, got, want)
		}
	}
}

// 拒绝结束非 gost 进程（PID 复用保护）。
func TestTerminateRejectsNonGost(t *testing.T) {
	// 用当前测试进程冒充：它的 exe 不是 gost.exe
	if err := Terminate(os.Getpid()); err == nil {
		t.Fatal("不应允许结束非 gost 进程")
	}
	if !ProcessAlive(os.Getpid()) {
		t.Fatal("测试进程被误杀")
	}
}

func TestIsGostExeCaseInsensitive(t *testing.T) {
	for _, name := range []string{"gost.exe", "GOST.EXE", "Gost.Exe"} {
		if !isGostExe(name) {
			t.Errorf("isGostExe(%q) = false", name)
		}
	}
	if isGostExe("gost2.exe") {
		t.Error("isGostExe(gost2.exe) = true")
	}
}

func TestPortOwnerFindsIPv4AndIPv6(t *testing.T) {
	cases := []struct {
		network string
		addr    string
	}{
		{"tcp4", "127.0.0.1:0"},
		{"tcp6", "[::1]:0"},
	}
	for _, tc := range cases {
		t.Run(tc.network, func(t *testing.T) {
			ln, err := net.Listen(tc.network, tc.addr)
			if err != nil {
				t.Skipf("listen %s: %v", tc.network, err)
			}
			defer ln.Close()
			port := ln.Addr().(*net.TCPAddr).Port

			// netstat 可能比监听建立稍晚可见。
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if pid, _ := PortOwner(port); pid == os.Getpid() {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			pid, _ := PortOwner(port)
			t.Fatalf("PortOwner(%d) = %d, want %d", port, pid, os.Getpid())
		})
	}
}

// 自定义文件名（不叫 gost.exe）也必须能通过预期路径结束。
func TestTerminateWithExpectedBinary(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestWindowsHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()

	time.Sleep(100 * time.Millisecond)
	if err := Terminate(cmd.Process.Pid, exe); err != nil {
		t.Fatalf("Terminate with expected path: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("helper process did not exit")
	}
}

func TestWindowsHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}
