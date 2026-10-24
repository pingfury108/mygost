package main

import (
	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
)

// ─── 设计系统组件（全站复用）──────────────────────────────────────

// card 是内容卡片的统一容器：圆角大于行，形成层级。
func card(c *ui.Context) ui.Element {
	return ui.Box(c).FillWidth().Padding(sp3+2, sp4).Radius(rCard).Background(c.Theme().Surface)
}

// sectionTitle 是卡片内的小节标题：字号小于正文、字重高于正文，靠同级对比识别。
func sectionTitle(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(fsCaption).Bold().
		TextColor(c.Theme().TextMuted).LetterSpacing(0.4)
}

// kvRow 是键值行：键固定窄列，值占满并可截断，中文不再挤压。
func kvRow(c *ui.Context, key, val string) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(sp3).AlignItems(ui.Start).Children(func() {
		ui.Text(c, key).FontSize(fsBody).TextColor(t.TextMuted).Width(72).Shrink(0)
		ui.Text(c, val).FontSize(fsBody).Grow(1).Ellipsis("end")
	})
}

// settingRow 是横向设置行：左标签+描述，右控件。控件是开关一类的小元素。
func settingRow(c *ui.Context, label, desc string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp3).Children(func() {
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, label).FontSize(fsBody)
			if desc != "" {
				ui.Text(c, desc).FontSize(fsCaption).TextColor(t.TextMuted)
			}
		})
		if control != nil {
			control()
		}
	})
}

// settingField 是纵向设置行：标签+描述在上，控件占满整行。适合输入框。
func settingField(c *ui.Context, label, desc string, control func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(sp2).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, label).FontSize(fsBody)
			if desc != "" {
				ui.Text(c, desc).FontSize(fsCaption).TextColor(t.TextMuted)
			}
		})
		if control != nil {
			control()
		}
	})
}

// divider 是卡片内的分隔线：与内容左边距对齐，不贴卡片边缘。
func divider(c *ui.Context) {
	t := c.Theme()
	ui.Box(c).FillWidth().Height(1).Background(t.Border).Opacity(0.6)
}

// statusDot 是状态圆点。
func statusDot(c *ui.Context, color ui.Color, size float32) {
	ui.Box(c).Width(size).Height(size).Radius(size / 2).Background(color)
}

// iconButton 是只有图标的方形点击热区，尺寸固定以便对齐。
func iconButton(c *ui.Context, glyph string) ui.Element {
	t := c.Theme()
	return ui.ButtonBase(c).Width(24).Height(24).Radius(rCtl).Center().
		Cursor(ui.CursorPointer).Children(func() {
		ui.Text(c, glyph).FontSize(fsBody).TextColor(t.TextMuted)
	})
}

// subPage 是子页面骨架：返回按钮 + 标题，正文由 body 渲染。
func subPage(c *ui.Context, title string, back func(), body func(*ui.Context)) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).
			Padding(sp3, sp4, sp2).Children(func() {
			btn := ui.ButtonBase(c).Width(28).Height(28).Radius(rCtl).Center().
				Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, "←").FontSize(fsBody).TextColor(t.TextMuted)
			})
			ui.Text(c, title).FontSize(fsTitle).Bold().Grow(1)
			if btn.Clicked() {
				back()
			}
		})
		body(c)
	})
}

// ─── 根视图 ──────────────────────────────────────────────────────

// view 是窗口根视图：安装应用主题，再渲染主界面或推入式子页面。
func (a *app) view(c *ui.Context) {
	// 主题：跟随系统深浅色，首次与切换时各安装一次
	if t := c.Theme(); !a.themeApplied || a.themeDark != t.Dark {
		c.SetTheme(appTheme(t.Dark))
		a.themeApplied, a.themeDark = true, t.Dark
	}

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
