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
)

// 图标/状态点尺寸
const (
	dotSm float32 = 8
	dotMd float32 = 14
)

// appTheme 由系统默认主题派生出应用主题，保留系统深浅色。
func appTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	// 间距单位与圆角跟 token 对齐
	t.Spacing = sp1
	t.Radius = rCtl
	return t
}

// accentSoft 是选中行的底色：主色的极淡版本，替代描边。
func accentSoft(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(59, 130, 246, 0.16)
	}
	return ui.RGBA(37, 99, 235, 0.07)
}

// successHalo 是"已连接"按钮外的光环：只声明状态，不参与文字语义。
func successHalo(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(34, 197, 94, 0.18)
	}
	return ui.RGBA(22, 163, 74, 0.16)
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
