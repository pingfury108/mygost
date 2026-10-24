package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"os/exec"

	"mygost/internal/geo"
	"mygost/internal/profiles"
)

// 端到端：真实下载 CN 列表 → 启动 geo 插件 → 生成含分流的 YAML。
// 本机装有 gost 时顺带校验 gost 能接受该配置；缺件则跳过。
func TestSplitE2E(t *testing.T) {
	home, _ := os.UserHomeDir()
	plug := filepath.Join(home, "go", "bin", "gost-geo-plugin")
	gost := filepath.Join(home, "Library", "Application Support", "mygost", "bin", "gost")
	for _, p := range []string{plug, gost} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("missing %s", p)
		}
	}

	m := geo.NewManager(t.TempDir(), 8111)
	n, err := m.UpdateIPList(context.Background(), geo.DefaultIPListURL)
	if err != nil {
		t.Skipf("offline: %v", err)
	}
	t.Logf("ip entries: %d", n)
	if err := m.InstallFromPath(plug); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if err := m.WaitReady(ctx, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	p := &profiles.Profile{
		Name: "e2e", Kind: profiles.KindProxy, Listen: ":18026", Proto: "http",
		UpEnabled: true, UpProto: "sshd", UpAddr: "149.129.41.181:8081",
		UpUser: "gost", UpPass: "xxx",
	}
	y, err := p.BuildYAMLSplit(profiles.SplitConfig{
		Enabled:         true,
		PluginAddr:      m.Addr(),
		PrivateMatchers: geo.PrivateMatchers,
	})
	if err != nil {
		t.Fatal(err)
	}
	yml := filepath.Join(t.TempDir(), "gost.yml")
	if err := os.WriteFile(yml, y, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("generated yaml:\n%s", y)

	// gost 以该配置启动 3 秒不死 = 配置合法（插件可达、bypass 结构正确）
	_ = strings.TrimSpace("")
	cmd, err := gostRun(gost, yml)
	if err != nil {
		t.Skipf("gost run helper: %v", err)
	}
	cmd.Stderr = os.Stderr // 让 gost 的报错直接可见
	if err := cmd.Start(); err != nil {
		t.Skipf("gost start failed: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("gost exited early: %v", err)
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func gostRun(bin, yml string) (*exec.Cmd, error) {
	return exec.Command(bin, "-C", yml), nil
}
