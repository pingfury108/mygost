package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 设置页：分流开时有 IP 列表行；关时没有。
func TestSettingsSplitCard(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"

	tt := ui.NewTester(a.view, 430, 900)
	if !tt.HasText(tr("国内直连", "Direct for CN")) {
		t.Errorf("split switch missing, texts %q", tt.Texts())
	}
	if tt.HasText(tr("IP 列表", "IP List")) {
		t.Errorf("IP list row should hide when split off, texts %q", tt.Texts())
	}

	a.sSplit = true
	tt2 := ui.NewTester(a.view, 430, 900)
	if !tt2.HasText(tr("尚未下载", "Not downloaded")) {
		t.Errorf("IP list should show not-downloaded state, texts %q", tt2.Texts())
	}
}
