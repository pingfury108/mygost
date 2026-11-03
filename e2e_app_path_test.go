package main

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mygost/internal/geo"
	"mygost/internal/proc"
	"mygost/internal/profiles"
)

// 走 app 的真实代码路径（geo.Manager + profiles.BuildYAMLSplit + proc.Manager），
// 用临时端口，不触碰已安装实例。
func TestRealPathSplitE2E(t *testing.T) {
	home, _ := os.UserHomeDir()
	gostBin := filepath.Join(home, "Library", "Application Support", "mygost", "bin", "gost")
	if _, err := os.Stat(gostBin); err != nil {
		t.Skip("gost 未安装")
	}
	if len(geo.PluginBytes()) == 0 {
		t.Skip("当前平台未嵌入 splitd")
	}

	// 1) 用 geo.Manager 下载真实的两份列表（与设置页「更新」同一路径）
	m := geo.NewManager(t.TempDir(), 18120)
	ctx := context.Background()
	ips, err := m.UpdateIPList(ctx, geo.DefaultIPListURL)
	if err != nil {
		t.Skipf("ip list offline: %v", err)
	}
	domains, err := m.UpdateDomainList(ctx, geo.DefaultDomainListURL)
	if err != nil {
		t.Skipf("domain list offline: %v", err)
	}
	t.Logf("下载完成：%d IP / %d 域名", ips, domains)

	// 2) 启动插件（会自动解压嵌入的二进制）
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if err := m.WaitReady(ctx, 20*time.Second); err != nil {
		t.Fatalf("插件未就绪: %v", err)
	}
	t.Log("插件已就绪")

	// 3) 生成配置（与 app 相同调用）
	p := &profiles.Profile{
		Kind: profiles.KindProxy, Listen: ":18121", Proto: "http",
		UpEnabled: true, UpProto: "http", UpAddr: "127.0.0.1:59992",
	}
	y, err := p.BuildYAMLSplit(profiles.SplitConfig{
		Enabled: true, PluginAddr: m.Addr(),
		PrivateMatchers: geo.PrivateMatchers, DebugLog: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	yml := filepath.Join(dir, "g.yml")
	os.WriteFile(yml, y, 0o644)

	// 4) 用 proc.Manager 起 gost（与 app 相同路径）
	pm := proc.NewManager(nil)
	if err := pm.Start(gostBin, yml); err != nil {
		t.Fatal(err)
	}
	defer pm.Stop()
	if err := pm.WaitStable(1200 * time.Millisecond); err != nil {
		t.Fatalf("gost 启动失败: %v", err)
	}

	// 5) 通过代理发请求
	pu, _ := url.Parse("http://127.0.0.1:18121")
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(pu)},
		Timeout:   3 * time.Second,
	}
	for _, target := range []string{"http://baidu.com/", "http://gateway.icloud.com/"} {
		resp, err := client.Get(target)
		if err == nil {
			resp.Body.Close()
		}
	}
	time.Sleep(700 * time.Millisecond)

	// 6) 验证 route 行：baidu 无节点（直连），icloud 有节点（代理）
	logs := strings.Join(pm.Lines(0), "\n")
	directOK := strings.Contains(logs, "route(retry=0) baidu.com:80")
	proxyOK := strings.Contains(logs, "> gateway.icloud.com:80")
	if !directOK {
		t.Errorf("baidu 应直连，日志片段:\n%s", tail(logs, 2000))
	}
	if !proxyOK {
		t.Errorf("icloud 应走代理，日志片段:\n%s", tail(logs, 2000))
	}
	t.Logf("验证通过：baidu 直连=%v，icloud 代理=%v", directOK, proxyOK)
	_ = net.Dial
	_ = exec.Command
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
