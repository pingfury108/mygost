package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
	"mygost/internal/reqlog"
)

// viewRequests 是日志页的「请求」视图：每个请求一行，含去向与失败原因。
func (a *app) viewRequests(c *ui.Context) {
	t := c.Theme()
	entries := a.reqs.Entries(0)
	if q := strings.TrimSpace(a.logFilter); q != "" {
		kept := entries[:0:0]
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Target), strings.ToLower(q)) ||
				strings.Contains(strings.ToLower(e.Err), strings.ToLower(q)) {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	if a.reqOnlyFailed {
		kept := entries[:0:0]
		for _, e := range entries {
			if e.Err != "" {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	// 只渲染最近 200 条
	if len(entries) > 200 {
		entries = entries[len(entries)-200:]
	}

	ui.Column(c).Fill().Padding(sp4).Gap(sp3).Children(func() {
		// 工具栏
		ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.logFilter).Grow(1).Placeholder(tr("过滤目标或错误…", "Filter target or error…"))
			ui.Toggle(c, &a.reqOnlyFailed, tr("仅失败", "Failed only")).FontSize(fsCaption)
			ui.Button(c, tr("清空", "Clear")).OnClick(func() { a.reqs.Clear() })
		})

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).Children(func() {
			ui.Text(c, fmt.Sprintf(tr("%d 条", "%d entries"), len(entries))).
				FontSize(fsCaption).TextColor(t.TextMuted).Grow(1)
			if !a.reqs.Enabled() {
				ui.Text(c, tr("未记录（需开启调试日志）", "Not recording (needs debug log)")).
					FontSize(fsCaption).TextColor(t.Warning)
			}
		})

		// 列表
		ui.Box(c).FillWidth().Grow(1).Radius(rCard).Background(t.Surface).
			Padding(sp3).Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Gap(1).Children(func() {
					if len(entries) == 0 {
						ui.Column(c).Fill().Center().Gap(sp2).Padding(sp6).Children(func() {
							ui.Text(c, tr("暂无请求记录", "No requests yet")).
								FontSize(fsBody).TextColor(t.TextMuted)
							ui.Text(c, tr("连接后发起的请求会显示在这里", "Requests appear here once connected")).
								FontSize(fsCaption).TextColor(t.TextMuted)
						})
						return
					}
					for _, e := range entries {
						a.requestRow(c, e)
					}
				})
			})
		})
	})
}

// requestRow 渲染一条请求：时间 · 目标 · 去向 · 节点/错误。
func (a *app) requestRow(c *ui.Context, e reqlog.Entry) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Padding(2, 2).Children(func() {
		ui.Text(c, e.At.Format("15:04:05")).FontSize(fsCaption).
			Font("monospace").TextColor(t.TextMuted).Width(58)

		// 去向标签：直连（灰）/ 代理（蓝）/ 失败（红）
		label, color := tr("直连", "Direct"), t.TextMuted
		switch {
		case e.Err != "":
			label, color = tr("失败", "Failed"), t.Danger
		case e.Decision == reqlog.Proxied:
			label, color = tr("代理", "Proxy"), t.Accent
		}
		ui.Text(c, label).FontSize(fsCaption).TextColor(color).Width(30)

		ui.Text(c, e.Target).FontSize(fsBody).Grow(1).SingleLine()

		// 右侧：错误原因优先，其次节点
		if e.Err != "" {
			ui.Text(c, e.Err).FontSize(fsCaption).TextColor(t.Danger).MaxLines(1)
		} else if e.Node != "" {
			ui.Text(c, e.Node).FontSize(fsCaption).TextColor(t.TextMuted).MaxLines(1)
		}
	})
}

// requestCount 返回请求条数（用于日志页标签）。
func (a *app) requestCount() int { return a.reqs.Len() }

// feedRequest 把一行原始日志喂给请求解析器（由 proc 的日志回调调用）。
func (a *app) feedRequest(line string) {
	if a.reqs == nil {
		return
	}
	before := a.reqs.Len()
	a.reqs.Feed(line)
	if a.reqs.Len() != before {
		// 有新请求：让界面刷新（节流由 proc 的 notifySoon 负责）
		if a.win != nil {
			a.win.Invalidate()
		}
	}
}

// formatLatency 把延迟格式化（当前未从日志采集，保留给后续扩展）。
func formatLatency(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Round(time.Millisecond).String()
}

var _ = proc.Running
