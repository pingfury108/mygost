package geo

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mygost/internal/splitbin"
)

// 端口被其他程序占用时应自动换端口，而不是启动失败。
func TestStartSwitchesPortWhenOccupied(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 18150)

	// 占住默认端口，模拟"8100 被其他服务用着"
	ln, err := net.Listen("tcp", "127.0.0.1:18150")
	if err != nil {
		t.Skipf("无法占用测试端口: %v", err)
	}
	defer ln.Close()

	if err := m.ensurePort(); err != nil {
		t.Fatalf("ensurePort: %v", err)
	}
	if m.Addr() == "127.0.0.1:18150" {
		t.Fatalf("端口被占用时应换端口，实际仍为 %s", m.Addr())
	}
}

// 端口空闲时保持原端口。
func TestEnsurePortKeepsFreePort(t *testing.T) {
	m := NewManager(t.TempDir(), 18151)
	if err := m.ensurePort(); err != nil {
		t.Fatal(err)
	}
	if got := m.Addr(); got != "127.0.0.1:18151" {
		t.Errorf("空闲端口不该换，得到 %s", got)
	}
}

// 地址上已有实例时 Start 应直接复用，不新拉进程。
//
// 这是 app 重启后的关键场景：自有 cmd 为 nil，但上次的 splitd 还在监听。
func TestStartReusesExistingInstance(t *testing.T) {
	dir := t.TempDir()
	writeRuleFiles(t, dir)

	m := NewManager(dir, 18152)
	// 假装"上次遗留的插件"正监听
	ln, err := net.Listen("tcp", "127.0.0.1:18152")
	if err != nil {
		t.Skipf("无法占用测试端口: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start 应复用已有实例: %v", err)
	}
	if !m.Running() {
		t.Log("未拉起自有进程（正确：复用了外部实例）")
	}
	if !m.Alive() {
		t.Error("Alive 应为 true")
	}
	st := m.Status()
	if !st.Alive || st.Addr != "127.0.0.1:18152" {
		t.Errorf("Status = %+v", st)
	}
}

// Alive 在无人监听时为 false。
func TestAliveFalseWhenNothingListens(t *testing.T) {
	m := NewManager(t.TempDir(), 18153) // 该端口无人监听
	if m.Alive() {
		t.Error("无进程监听时 Alive 应为 false")
	}
}

// Stop 不应误杀复用的外部实例。
func TestStopLeavesExternalInstance(t *testing.T) {
	dir := t.TempDir()
	writeRuleFiles(t, dir)
	m := NewManager(dir, 18154)

	ln, err := net.Listen("tcp", "127.0.0.1:18154")
	if err != nil {
		t.Skipf("无法占用测试端口: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.Stop()
	if !m.Alive() {
		t.Error("外部实例应仍在监听（Stop 只杀自有进程）")
	}
	// ForceStop 则应报告仍有外部实例
	if err := m.ForceStop(); err == nil {
		t.Error("ForceStop 应提示端口上仍有外部实例")
	}
}

// writeRuleFiles 写最小可用的规则文件，让 Start 通过 Ready 检查。
func writeRuleFiles(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "geoip-cn.txt"), []byte("1.0.1.0/24\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "geosite-cn.txt"), []byte("baidu.com\n"), 0o644)
}

// 回归：调用方 ctx 取消后插件必须继续运行。
//
// 曾经 Start 用 context.WithCancel(调用方 ctx)，而 ensureGeo 里
// defer cancel() 会立刻取消它 → 插件被 exec.CommandContext 杀掉，
// 表现为"启动成功但端口无监听"。
func TestPluginSurvivesCallerContextCancel(t *testing.T) {
	dir := t.TempDir()
	writeRuleFiles(t, dir)
	m := NewManager(dir, 18155)
	if len(splitbinBin()) == 0 {
		t.Skip("当前平台未嵌入插件")
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel() // 模拟 ensureGeo 返回时的 defer cancel()
	t.Cleanup(func() { m.ForceStop() })

	time.Sleep(500 * time.Millisecond) // 给子进程被杀的时机
	if !m.Running() {
		t.Fatal("调用方 ctx 取消后插件不该被终止")
	}
	if !m.Alive() {
		t.Fatal("插件应仍在监听")
	}
}

// 回归：进程已退出时 Running 必须为 false，否则会永远认为插件健康、
// 再也不重试（Split 的 gRPC 调用全部失败的根因之一）。
func TestRunningFalseAfterExit(t *testing.T) {
	dir := t.TempDir()
	writeRuleFiles(t, dir)
	m := NewManager(dir, 18156)
	if len(splitbinBin()) == 0 {
		t.Skip("当前平台未嵌入插件")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	m.Stop()
	time.Sleep(400 * time.Millisecond)
	if m.Running() {
		t.Error("进程已停止，Running 应为 false")
	}
	if m.Alive() {
		t.Error("进程已停止且无人监听，Alive 应为 false")
	}
}

// splitbinBin 暴露嵌入的插件字节（测试用）。
func splitbinBin() []byte { return splitbin.Plugin() }
