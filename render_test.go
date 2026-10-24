package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
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
	a.page = "settings"
	render("p_settings")
	a.page = "logs"
	render("p_logs")
	a.startNew()
	a.formKind = 2 // YAML 标签
	a.syncFormKind()
	render("p_yaml")

	a.startNew()
	a.form.Listen = ":8027"
	a.form.UpEnabled = true
	a.form.UpProto = "sshd"
	a.form.UpAddr = "149.129.41.181:8081"
	a.form.UpUser = "gost"
	a.form.UpPass = "secretpass"
	a.form.Name = "hk"
	render("p_editor")
}
