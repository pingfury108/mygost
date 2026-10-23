package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	st, err := settings.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := profiles.LoadStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &app{
		page: "main",
		st:   st,
		ps:   ps,
		pm:   proc.NewManager(nil),
	}
}

// 视图在无窗口的 Tester 中构建，验证各页面可渲染且文本正确。

func TestMainEmpty(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("已停止") {
		t.Errorf("should show stopped state, texts %q", tt.Texts())
	}
	if !tt.HasText("还没有配置") {
		t.Errorf("empty list should guide to add, texts %q", tt.Texts())
	}
	if !tt.HasText("gost 未安装") {
		t.Errorf("missing binary should show download banner, texts %q", tt.Texts())
	}
}

func TestMainWithProfile(t *testing.T) {
	a := newTestApp(t)
	a.binFound = true
	if _, err := a.ps.Save(&profiles.Profile{
		Name: "测试代理", Kind: profiles.KindProxy, Listen: ":8080", Proto: "http",
	}); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("测试代理") {
		t.Errorf("profile row should show name, texts %q", tt.Texts())
	}
	if !tt.HasText(":8080") {
		t.Errorf("profile row should show listen addr, texts %q", tt.Texts())
	}
	if !tt.HasText("连接") {
		t.Errorf("profile row should offer connect, texts %q", tt.Texts())
	}
}

func TestLogsView(t *testing.T) {
	a := newTestApp(t)
	a.page = "logs"
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("暂无日志") {
		t.Errorf("logs page should render empty state, texts %q", tt.Texts())
	}
}

func TestSettingsView(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("保存设置") {
		t.Errorf("settings page should render, texts %q", tt.Texts())
	}
	if !tt.HasText("gost 程序") {
		t.Errorf("settings should group by card, texts %q", tt.Texts())
	}
}

func TestEditorView(t *testing.T) {
	a := newTestApp(t)
	a.startNew()
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("保存") {
		t.Errorf("editor should render form actions, texts %q", tt.Texts())
	}
	if !tt.HasText("监听地址") {
		t.Errorf("proxy form should show listen field, texts %q", tt.Texts())
	}
}
