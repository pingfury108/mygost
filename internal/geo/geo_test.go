package geo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseGeositeJSON(t *testing.T) {
	body := `{"rules":[{"domain_suffix":["baidu.com","qq.com","baidu.com"]},{"domain_suffix":["alibaba"]}]}`
	got, err := ParseGeositeJSON(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"baidu.com", "qq.com", "alibaba"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestParseGeositeInvalid(t *testing.T) {
	if _, err := ParseGeositeJSON(strings.NewReader("{bad")); err == nil {
		t.Error("invalid json should error")
	}
}

func TestUpdateListsFromFile(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 8101)

	ips := filepath.Join(dir, "ips.txt")
	os.WriteFile(ips, []byte("# c\n1.0.1.0/24\n\n2.0.0.0/8\n"), 0o644)
	n, err := m.UpdateIPList(context.Background(), "file://"+ips)
	if err != nil || n != 2 {
		t.Fatalf("ip list: %d %v", n, err)
	}

	geo := filepath.Join(dir, "cn.json")
	os.WriteFile(geo, []byte(`{"rules":[{"domain_suffix":["baidu.com","qq.com"]}]}`), 0o644)
	dn, err := m.UpdateDomainList(context.Background(), "file://"+geo)
	if err != nil || dn != 2 {
		t.Fatalf("domain list: %d %v", dn, err)
	}

	if !m.Ready() {
		t.Error("both lists present should be ready")
	}
	if m.CountIP() != 2 || m.CountDomains() != 2 {
		t.Errorf("counts: %d %d", m.CountIP(), m.CountDomains())
	}
}

func TestNotReadyWithoutLists(t *testing.T) {
	m := NewManager(t.TempDir(), 8102)
	if m.Ready() {
		t.Error("empty dir should not be ready")
	}
	if err := m.Start(context.Background()); err == nil {
		t.Error("start should fail without lists")
	}
}

// 零安装：嵌入的插件应被解压并能启动服务。
func TestEmbeddedPluginAutoInstall(t *testing.T) {
	if len(PluginBytes()) == 0 {
		t.Skip("当前平台未嵌入插件")
	}
	m := NewManager(t.TempDir(), 8103)
	if err := os.WriteFile(m.IPListPath(), []byte("1.0.1.0/24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.DomainListPath(), []byte("baidu.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if err := m.WaitReady(ctx, 15*time.Second); err != nil {
		t.Fatalf("plugin not ready: %v", err)
	}
	if st, err := os.Stat(m.PluginPath()); err != nil || st.Size() == 0 {
		t.Fatalf("plugin not extracted: %v", err)
	}
}
