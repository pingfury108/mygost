package geo

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateIPListAndCount(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 8101)

	// 本地文件模拟下载源
	src := filepath.Join(dir, "src.txt")
	body := "# comment\n1.0.1.0/24\n\n2.0.0.0/8\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := m.UpdateIPList(context.Background(), "file://"+src)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
	if !m.ListReady() {
		t.Error("list should be ready")
	}
	if got := m.CountLines(); got != 2 {
		t.Errorf("CountLines = %d", got)
	}
	if m.UpdateAt().IsZero() {
		t.Error("UpdateAt should be set")
	}
}

func TestManagerStartStop(t *testing.T) {
	if _, err := os.Stat(pluginDevPath()); err != nil {
		t.Skip("本地未安装 gost-geo-plugin（go install 后放到临时目录测试）")
	}
	dir := t.TempDir()
	m := NewManager(dir, 8102)
	// 把开发机的插件复制进去
	if err := m.InstallFromPath(pluginDevPath()); err != nil {
		t.Fatal(err)
	}
	// 写入一条私网规则即可启动
	if err := os.WriteFile(m.IPListPath(), []byte("10.0.0.0/8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if err := m.WaitReady(ctx, 5*time.Second); err != nil {
		t.Fatalf("plugin not ready: %v", err)
	}
	m.Stop()
	if m.Running() {
		t.Error("should not be running after Stop")
	}
}

func TestStartWithoutInstall(t *testing.T) {
	m := NewManager(t.TempDir(), 8103)
	if err := m.Start(context.Background()); err == nil {
		t.Error("start should fail when plugin is missing")
	}
}

func TestAddr(t *testing.T) {
	m := NewManager("x", 0)
	if m.Addr() != "127.0.0.1:8100" {
		t.Errorf("default addr %q", m.Addr())
	}
}

func TestPrivateMatchers(t *testing.T) {
	if len(PrivateMatchers) == 0 {
		t.Fatal("private matchers required")
	}
	for _, m := range []string{"127.0.0.0/8", "10.0.0.0/8"} {
		if net.ParseIP(m) == nil {
			if _, _, err := net.ParseCIDR(m); err != nil {
				t.Errorf("invalid matcher %q", m)
			}
		}
	}
}

func pluginDevPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "go", "bin", "gost-geo-plugin")
}
