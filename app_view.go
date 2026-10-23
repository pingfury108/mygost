package main

import (
	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
)

// ─── 设计系统组件（全站复用）──────────────────────────────────────

// card 返回统一卡片容器；并排时调用方加 .Grow(1)。
func card(c *ui.Context) ui.Element {
	return ui.Box(c).Fill().Padding(14, 16).Radius(10).Background(c.Theme().Surface)
}

// sectionTitle 卡片内小节标题。
func sectionTitle(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(13).Bold().TextColor(c.Theme().TextMuted)
}

// kvRow 统一键值行。
func kvRow(c *ui.Context, key, val string) {
	t := c.Theme()
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, key).TextColor(t.TextMuted).Width(64)
		ui.Text(c, val).Grow(1)
	})
}

// settingRow 统一设置行：左侧标签+描述，右侧控件。
func settingRow(c *ui.Context, label, desc string, control func()) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(12).Padding(4, 0).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, label)
			if desc != "" {
				ui.Text(c, desc).TextColor(t.TextMuted)
			}
		})
		if control != nil {
			control()
		}
	})
}

// statusDot 状态圆点。
func statusDot(c *ui.Context, color ui.Color, size float32) {
	ui.Box(c).Width(size).Height(size).Radius(size / 2).Background(color)
}

// subPage 子页面骨架：← 返回 + 标题 + 内容（窄窗口推入式导航）。
func subPage(c *ui.Context, title string, back func(), body func(*ui.Context)) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Padding(12, 16, 8).Children(func() {
			ui.Button(c, "←").OnClick(back)
			ui.Text(c, title).FontSize(17).Bold().Grow(1)
		})
		body(c)
	})
}

// ─── 根视图 ──────────────────────────────────────────────────────

// view 是窗口根视图：主界面与推入式子页面。
func (a *app) view(c *ui.Context) {
	// 展示排队的 toast
	for _, t := range a.pendingToasts {
		c.AddToast(t)
	}
	a.pendingToasts = nil

	switch a.page {
	case "logs":
		subPage(c, "日志", a.goMain, a.viewLogs)
	case "settings":
		subPage(c, "设置", a.goMain, a.viewSettings)
	case "editor":
		subPage(c, a.formTitle(), a.goMain, a.viewProfileForm)
	default:
		a.viewMain(c)
	}
}

func (a *app) goMain() { a.page = "main" }

// runningProfileID 返回正在运行档的 ID（未运行返回空）。
func (a *app) runningProfileID() string {
	if st, _, _ := a.pm.State(); st != proc.Running {
		return ""
	}
	return a.st.Get().Current
}
