package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
)

// viewLogs 日志页：过滤、复制、清空。
func (a *app) viewLogs(c *ui.Context) {
	t := c.Theme()
	lines := a.pm.Lines(0)
	shown := lines
	if q := strings.TrimSpace(a.logFilter); q != "" {
		shown = make([]string, 0, len(lines))
		for _, l := range lines {
			if strings.Contains(strings.ToLower(l), strings.ToLower(q)) {
				shown = append(shown, l)
			}
		}
	}
	// 只渲染最近 300 行，避免超长帧构建
	if len(shown) > 300 {
		shown = shown[len(shown)-300:]
	}

	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.logFilter).Grow(1).Placeholder("过滤关键词…")
			ui.Button(c, "复制").OnClick(func() {
				c.Services().WriteClipboard(a.pm.LogText())
			})
			ui.Button(c, "清空").OnClick(func() { a.pm.ClearLogs() })
		})
		ui.Text(c, fmt.Sprintf("%d / %d 行", len(shown), len(lines))).FontSize(12).TextColor(t.TextMuted)

		card(c).Grow(1).Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Fill().Gap(2).Children(func() {
					if len(shown) == 0 {
						ui.Column(c).Fill().Center().Gap(8).Children(func() {
							if state, _, _ := a.pm.State(); state == proc.Running {
								ui.Spinner(c)
								ui.Text(c, "等待日志输出…").TextColor(t.TextMuted)
							} else {
								ui.Text(c, "暂无日志").TextColor(t.TextMuted)
							}
						})
						return
					}
					for _, l := range shown {
						ui.Text(c, l)
					}
				})
			})
		})
	})
}
