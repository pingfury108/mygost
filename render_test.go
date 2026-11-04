package main

import (
	"fmt"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/metrics"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

// TestRenderPages 把各页面无窗口渲染成 PNG，供人工检查布局。
// 需显式开启：MYGOST_RENDER=1 go test -run TestRenderPages .
func TestRenderPages(t *testing.T) {
	if os.Getenv("MYGOST_RENDER") == "" {
		t.Skip("设 MYGOST_RENDER=1 后运行")
	}
	a := newTestApp(t)
	a.binFound = true
	a.binVer = "gost v3.3.1"
	p1 := &profiles.Profile{Name: "hk", Kind: profiles.KindProxy, Listen: ":8027", Proto: "http",
		UpEnabled: true, UpProto: "sshd", UpAddr: "1.2.3.4:8081", UpUser: "gost"}
	p2 := &profiles.Profile{Name: "nas", Kind: profiles.KindForward, Listen: ":2222",
		FwdProto: "tcp", Targets: "192.168.1.10:22"}
	a.ps.Save(p1)
	a.ps.Save(p2)
	a.st.Update(func(s *settings.Settings) { s.Current = p1.ID })

	render := func(name string) {
		img := ui.Render(a.view, 430, 660, 2)
		f, _ := os.Create("/tmp/" + name + ".png")
		png.Encode(f, img)
		f.Close()
	}

	a.page = "main"
	render("p_main")

	// 未连接 + 分流开启：主界面应出现分流状态卡（点击进设置）
	os.WriteFile(a.geoM.IPListPath(), []byte("1.2.3.0/24\n"), 0o644)
	os.WriteFile(a.geoM.DomainListPath(), []byte("baidu.com\n"), 0o644)
	a.st.Update(func(s *settings.Settings) { s.Split = true })
	a.sSplit = true
	render("p_main_split")
	a.st.Update(func(s *settings.Settings) { s.Split = false })
	a.sSplit = false

	// 已连接态：接管自身进程伪装 Running，检查速率卡与底部状态行
	_ = a.pm.Attach(os.Getpid(), time.Now().Add(-2*time.Hour))
	a.metricsOK = true
	a.rates = metrics.Rates{Up: 1.2e6, Down: 8.4e6, TotalUp: 142e6, TotalDown: 1.2e9}
	render("p_main_on")

	// 4 个配置 + 连接态：验证列表不被速率卡挤掉（可滚动且末条可见）
	more := []*profiles.Profile{
		{Name: "al-hk", Kind: profiles.KindProxy, Listen: ":8026", Proto: "http",
			UpEnabled: true, UpProto: "tcp", UpAddr: "47.109.61.89:5903"},
		{Name: "yz", Kind: profiles.KindProxy, Listen: ":8026", Proto: "http",
			UpEnabled: true, UpProto: "tcp", UpAddr: "127.0.0.1:1080"},
	}
	for _, p := range more {
		a.ps.Save(p)
	}
	render("p_main_on_many")

	// 8 个配置 + 连接态：验证 N 条时其他元素不被压缩、列表可滚
	for i := 0; i < 4; i++ {
		a.ps.Save(&profiles.Profile{
			Name: fmt.Sprintf("extra-%d", i+1), Kind: profiles.KindProxy,
			Listen: ":900" + fmt.Sprint(i), Proto: "http",
			UpEnabled: true, UpProto: "tcp", UpAddr: fmt.Sprintf("10.0.0.%d:1080", i+1),
		})
	}
	render("p_main_on_many8")

	a.page = "settings"
	render("p_settings")
	a.st.Update(func(s *settings.Settings) { s.Split = true })
	a.sSplit = true
	os.WriteFile(a.geoM.IPListPath(), []byte("1.2.3.0/24\n"), 0o644)
	os.WriteFile(a.geoM.DomainListPath(), []byte("baidu.com\n"), 0o644)
	render("p_settings_split")
	a.st.Update(func(s *settings.Settings) { s.Split = false })
	a.sSplit = false
	a.page = "logs"
	render("p_logs")
	// 请求视图：喂几条真实格式的日志再看
	a.logView = 1
	a.reqs.Feed(`{"time":"2026-11-03T21:14:32.090+08:00","level":"debug","msg":"bypass 192.168.1.10:8080","kind":"handler","sid":"a1"}`)
	a.reqs.Feed(`{"time":"2026-11-03T21:14:32.092+08:00","level":"debug","msg":"proxy 149.129.41.181:8081","kind":"handler","sid":"a2"}`)
	a.reqs.Feed(`{"time":"2026-11-03T21:14:32.094+08:00","level":"debug","msg":"route(retry=0) node-0@149.129.41.181:8081 > gateway.icloud.com:443","kind":"handler","sid":"a2"}`)
	a.reqs.Feed(`{"time":"2026-11-03T21:14:33.100+08:00","level":"debug","msg":"proxy 149.129.41.181:8081","kind":"handler","sid":"a3"}`)
	a.reqs.Feed(`{"time":"2026-11-03T21:14:33.102+08:00","level":"debug","msg":"route(retry=0) dial tcp 149.129.41.181:8081: connect: connection refused","kind":"handler","sid":"a3"}`)
	render("p_requests")
	a.startNew()
	a.formKind = 2 // YAML 标签
	a.syncFormKind()
	render("p_yaml")

	a.startNew()
	a.formKind = 1 // 端口转发标签
	a.form.Listen = ":2222"
	a.fwdTargets = []string{"192.168.1.10:22", "192.168.1.11:22"}
	a.form.Name = "家里 NAS"
	render("p_forward")

	a.startNew()
	a.form.Listen = ":8027"
	a.form.UpEnabled = true
	a.upSeg = 1
	a.form.UpProto = "sshd"
	a.form.UpAddr = "149.129.41.181:8081"
	a.form.UpUser = "gost"
	a.form.UpPass = "secretpass"
	a.form.Name = "hk"
	render("p_editor")
}
