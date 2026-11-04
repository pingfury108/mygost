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
		ui.Text(c, val).FontSize(fsBody).Grow(1).SingleLine()
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

// statusPill 是顶部状态胶囊：圆点 + 文本，底色用淡色面而非描边。
func statusPill(c *ui.Context, dot ui.Color, parts ...string) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(sp2).
		Padding(sp1+2, sp3).Radius(999).Background(t.SurfaceHover).Children(func() {
		statusDot(c, dot, 7)
		for i, s := range parts {
			tx := ui.Text(c, s).FontSize(fsCaption).TextColor(t.TextMuted)
			if i > 0 {
				tx.Font("monospace") // 时长等数字部分用等宽
			}
		}
	})
}

// tagPill 是行内小标签：配置类型（代理/端口转发），选中行用主色淡底。
func tagPill(c *ui.Context, label string, soft bool) {
	t := c.Theme()
	bg, fg := t.SurfaceHover, t.TextMuted
	if soft {
		bg, fg = accentSoft(t), t.Accent
	}
	ui.Box(c).Padding(2, sp2).Radius(5).Background(bg).
		Children(func() {
			ui.Text(c, label).FontSize(fsCaption).TextColor(fg)
		})
}

// smallButton 是强调色淡底的小按钮（更新、下载），比默认按钮更轻。
func smallButton(c *ui.Context, label string, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Padding(sp1+2, sp3).Radius(rCtl).
		Background(accentSoft(t)).Cursor(ui.CursorPointer).Children(func() {
		ui.Text(c, label).FontSize(fsCaption).Bold().TextColor(t.Accent)
	})
	if onClick != nil && b.Clicked() {
		onClick()
	}
}

// sectionHeader 是卡片外的分节标题（对照设计稿：标题不在卡内）。
func sectionHeader(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(fsCaption).Bold().
		TextColor(c.Theme().TextMuted).Padding(0, sp1)
}

// formCtlW 是表单右侧控件的统一宽度：输入框与分段切换共用，
// 保证同一列右边缘对齐（宽度不一致时视觉上参差不齐）。
const formCtlW float32 = 176

// upCtlW 是嵌套上游卡里两行的首列宽度：协议下拉与账号输入框同宽，
// 两行的竖向分界才对齐（Select 自身有内容最小宽，必须配 MinWidth(0)）。
const upCtlW float32 = 116

// formRow 是行式表单行：标签+说明左，控件右（对照设计稿 A5/A7）。
func formRow(c *ui.Context, label, sub string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp3).Children(func() {
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, label).FontSize(fsBody)
			if sub != "" {
				ui.Text(c, sub).FontSize(fsCaption).TextColor(t.TextMuted)
			}
		})
		if control != nil {
			control()
		}
	})
}

// chipToggle 是胶囊开关（如"仅看失败"）：选中淡蓝底，未选灰底。
func chipToggle(c *ui.Context, val *bool, label string) {
	t := c.Theme()
	bg, fg := t.SurfaceHover, t.TextMuted
	if *val {
		bg, fg = accentSoft(t), t.Accent
	}
	b := ui.ButtonBase(c).Padding(sp1+2, sp3).Radius(10).Background(bg).
		Cursor(ui.CursorPointer).Children(func() {
		ui.Text(c, label).FontSize(fsCaption).Bold().TextColor(fg)
	})
	if b.Clicked() {
		*val = !*val
	}
}

// ghostButton 是描边按钮（复制一类次级操作）。
func ghostButton(c *ui.Context, label string, onClick func()) {
	t := c.Theme()
	b := ui.ButtonBase(c).Padding(sp1+2, sp3).Radius(10).
		Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorPointer).
		Children(func() {
			ui.Text(c, label).FontSize(fsCaption).TextColor(t.TextMuted)
		})
	if onClick != nil && b.Clicked() {
		onClick()
	}
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
		// body 必须占满剩余高度：否则它内部的 Fill/Grow 拿到的是内容高度，
		// 固定底栏会被推到窗口外面
		ui.Box(c).FillWidth().Grow(1).Children(func() { body(c) })
	})
}

// ─── 根视图 ──────────────────────────────────────────────────────

// view 是窗口根视图：安装应用主题，再渲染主界面或推入式子页面。
func (a *app) view(c *ui.Context) {
	// 主题：每帧显式安装。幂等且便宜，还保证无头渲染单帧也能拿到配色
	// （曾因"只装一次"导致后续帧回落到默认白底灰卡）。
	c.SetTheme(appTheme(c.Theme().Dark))

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

// segmentedFill 是等分宽度的分段控件：设计稿里编辑页三个 tab、日志两视图
// 都要求各段均分整行。
//
// 返回 true 表示用户选了新值。不用 ui.Segmented 的原因：它按文字宽度排布，
// 且 Grow(1) 只分配剩余空间，“经上游服务器”就比“直连目标”宽。
func segmentedFill(c *ui.Context, selected *int, labels ...string) bool {
	return segmentedIn(c, selected, 0, labels...)
}

// segmentedFixed 是表单里的分段切换：总宽与同行输入框一致，各段均分。
func segmentedFixed(c *ui.Context, selected *int, width float32, labels ...string) bool {
	return segmentedIn(c, selected, width, labels...)
}

// segmentedIn 是两种分段控件的共同实现；width 为 0 时占满整行。
//
// 用 Grid 而不是 Row+Grow：Grid 的列天然等宽，不受文字长度影响。
// 几何对照设计稿：轨道内边距/间隙 3、段高 30，选中块与轨道圆角相差 2。
func segmentedIn(c *ui.Context, selected *int, width float32, labels ...string) bool {
	t := c.Theme()
	g := ui.Grid(c).Columns(len(labels)).Gap(3).Padding(3).
		Radius(rCtl + 3).Background(t.SurfaceHover)
	if width > 0 {
		g = g.Width(width)
	} else {
		g = g.FillWidth()
	}
	changed := false
	cur := *selected
	g.Children(func() {
		for i, label := range labels {
			idx := i
			seg := ui.ButtonBase(c).Height(30).Radius(rCtl + 1).
				Center().Cursor(ui.CursorPointer)
			if idx == cur {
				seg = seg.Background(t.Surface)
			}
			seg.Children(func() {
				tx := ui.Text(c, label).FontSize(fsBody).SingleLine()
				if idx == cur {
					tx.Bold().TextColor(t.Text)
				} else {
					tx.TextColor(t.TextMuted)
				}
			})
			if seg.Clicked() && idx != cur {
				*selected = idx
				changed = true
			}
		}
	})
	return changed
}
