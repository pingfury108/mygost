package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// ─── 设计 token（全站唯一定义处）────────────────────────────────
//
// 间距：4pt 网格。
const (
	sp1 float32 = 4
	sp2 float32 = 8
	sp3 float32 = 12
	sp4 float32 = 16
	sp6 float32 = 24
)

// 圆角：控件 < 行 < 卡片，形成层级。圆形为直径的一半。
const (
	rCtl  float32 = 6
	rRow  float32 = 10
	rCard float32 = 12
)

// 字号：1.25 比例，避免相邻但不足以构成对比的字号。
const (
	fsCaption  float32 = 11 // 描述、次数、占位说明
	fsBody     float32 = 13 // 正文、设置标签、行摘要
	fsEmphasis float32 = 15 // 行标题、卡片标题
	fsTitle    float32 = 20 // 子页面标题
	fsDisplay  float32 = 28 // 空状态等页面级标题
	fsMono     float32 = 12 // 日志与代码
	fsStat     float32 = 20 // 速率卡等统计数字
)

// 图标/状态点尺寸
const (
	dotSm float32 = 8
	dotMd float32 = 14
)

// appTheme 由系统默认主题派生出应用主题，保留系统深浅色。
// 配色对照设计稿：浅色=方向 A「澄澈」，深色=方向 B「曜石」。
func appTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	// 间距单位与圆角跟 token 对齐
	t.Spacing = sp1
	t.Radius = rCtl

	if dark {
		// 曜石：近黑底 + 青色强调
		t.Background = ui.RGB(0x0C, 0x0E, 0x11)   // 画布
		t.Surface = ui.RGB(0x15, 0x18, 0x1D)      // 卡片
		t.SurfaceHover = ui.RGB(0x1B, 0x20, 0x28) // 内嵌控件/胶囊
		t.SurfacePressed = ui.RGB(0x23, 0x29, 0x32)
		t.Border = ui.RGBA(255, 255, 255, 0.07)
		t.Text = ui.RGB(0xE7, 0xEA, 0xEE)
		t.TextMuted = ui.RGB(0x8B, 0x92, 0x9C)
		t.Accent = ui.RGB(0x22, 0xD3, 0xEE)     // 青
		t.AccentText = ui.RGB(0x04, 0x23, 0x1B) // 青底上的深色文字
		t.Success = ui.RGB(0x10, 0xB9, 0x81)
		t.Warning = ui.RGB(0xFB, 0xBF, 0x24)
		t.Danger = ui.RGB(0xF8, 0x71, 0x71)
	} else {
		// 澄澈：灰底白卡 + 品牌蓝
		t.Background = ui.RGB(0xF6, 0xF7, 0xF9)   // 画布
		t.Surface = ui.RGB(0xFF, 0xFF, 0xFF)      // 卡片
		t.SurfaceHover = ui.RGB(0xED, 0xEF, 0xF3) // 内嵌控件/胶囊
		t.SurfacePressed = ui.RGB(0xE2, 0xE5, 0xEA)
		t.Border = ui.RGB(0xE7, 0xE9, 0xEF)
		t.Text = ui.RGB(0x17, 0x18, 0x1A)
		t.TextMuted = ui.RGB(0x7C, 0x80, 0x89)
		t.Accent = ui.RGB(0x25, 0x63, 0xEB) // 品牌蓝
		t.AccentText = ui.RGB(255, 255, 255)
		t.Success = ui.RGB(0x16, 0xA3, 0x4A)
		t.Warning = ui.RGB(0xD9, 0x77, 0x06)
		t.Danger = ui.RGB(0xDC, 0x26, 0x26)
	}
	return t
}

// accentSoft 是选中行的底色：主色的极淡版本，替代描边。
func accentSoft(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(34, 211, 238, 0.10)
	}
	return ui.RGBA(37, 99, 235, 0.08)
}

// successHalo 是"已连接"按钮光环的内环：只声明状态，不参与文字语义。
func successHalo(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(34, 197, 94, 0.18)
	}
	return ui.RGBA(22, 163, 74, 0.16)
}

// successHaloOuter 是光环的外环：更淡一层，形成双环衰减。
func successHaloOuter(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(34, 197, 94, 0.08)
	}
	return ui.RGBA(22, 163, 74, 0.07)
}

// accentHalo 是"可连接"按钮光环：蓝色主操作也有一圈柔光。
func accentHalo(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(34, 211, 238, 0.12)
	}
	return ui.RGBA(37, 99, 235, 0.10)
}

// dangerSoft 是危险提示条的底色。
func dangerSoft(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(239, 68, 68, 0.14)
	}
	return ui.RGBA(220, 38, 38, 0.06)
}

// warningSoft 是 gost 未安装横条的底色。
func warningSoft(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(245, 158, 11, 0.14)
	}
	return ui.RGBA(217, 119, 6, 0.07)
}

// 日志行着色：按关键字判断级别，仅用状态色，不做语法高亮。
func logLineColor(t *ui.Theme, line string) ui.Color {
	switch {
	case containsAny(line, "ERRO", "error", "Error", "FATA", "fatal", "failed", "失败"):
		return t.Danger
	case containsAny(line, "WARN", "warn", "Warning", "警告"):
		return t.Warning
	}
	return t.TextMuted
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
