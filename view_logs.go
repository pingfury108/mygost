package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
)

// viewLogs 日志页：搜索、复制、跟随最新；破坏性操作收进菜单。
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
	running := false
	if state, _, _ := a.pm.State(); state == proc.Running {
		running = true
	}

	ui.Column(c).Fill().Padding(sp4).Gap(sp3).Children(func() {
		// 工具栏：搜索占满，其余为图标/次级按钮
		ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Children(func() {
			ui.SearchField(c, &a.logFilter).Grow(1)
			ui.Button(c, "复制").OnClick(func() {
				c.Services().WriteClipboard(joinLines(shown)) // 所见即所得
			})
			menu := iconButton(c, "⋯")
			menu.Menu(func(m *ui.Menu) {
				if m.Item("复制全部日志").Chosen() {
					c.Services().WriteClipboard(a.pm.LogText())
				}
				if m.Item("清空日志").Chosen() {
					a.pm.ClearLogs()
				}
			})
		})

		// 日志容器
		ui.Box(c).FillWidth().Grow(1).Radius(rCard).Background(t.Surface).
			Padding(sp3).Children(func() {
			// 容器头：行数 + 跟随开关
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).
				Padding(0, 0, sp2).Children(func() {
				label := fmt.Sprintf("%d 行", len(shown))
				if len(shown) != len(lines) {
					label = fmt.Sprintf("%d / %d 行", len(shown), len(lines))
				}
				ui.Text(c, label).FontSize(fsCaption).TextColor(t.TextMuted).Grow(1)
				ui.Toggle(c, &a.logFollow, "跟随").FontSize(fsCaption)
			})
			ui.Box(c).FillWidth().Height(1).Background(t.Border).Opacity(0.6)

			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					if len(shown) == 0 {
						ui.Column(c).Fill().Center().Gap(sp2).Padding(sp6).Children(func() {
							if running {
								ui.Spinner(c)
								ui.Text(c, "等待日志输出…").FontSize(fsBody).TextColor(t.TextMuted)
							} else {
								ui.Text(c, "暂无日志").FontSize(fsBody).TextColor(t.TextMuted)
							}
						})
						return
					}
					// 等宽字体让时间戳对齐；按级别着色
					grew := len(shown) != a.logSeen
					a.logSeen = len(shown)
					for i, l := range shown {
						line := ui.Text(c, l).Font("monospace").FontSize(fsMono).
							TextColor(logLineColor(t, l))
						// 跟随模式：只在新增日志时滚到底，用户上翻不会被拽回
						if a.logFollow && grew && i == len(shown)-1 {
							line.ScrollIntoView()
						}
					}
				})
			})
		})
	})
}

// joinLines 拼接日志行（与 proc.LogText 一致，但用于过滤后的结果）。
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
