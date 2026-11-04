package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
	"mygost/internal/reqlog"
)

// logViewTabs 是日志两个视图共用的切换栏：否则切到请求视图后没地方切回来。
func (a *app) logViewTabs(c *ui.Context) {
	segmentedFill(c, &a.logView,
		tr("原始日志", "Raw"), fmt.Sprintf(tr("请求 (%d)", "Requests (%d)"), a.requestCount()))
}

// viewRequests 是日志页的「请求」视图：每个请求一行，含去向与失败原因。
func (a *app) viewRequests(c *ui.Context) {
	t := c.Theme()
	entries := a.requestEntries()
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

	// 汇总（按当前过滤后的列表统计）
	var nDir, nPrx, nErr, nUnknown int
	for _, e := range entries {
		switch {
		case e.Err != "":
			nErr++
		case e.Decision == reqlog.Proxied:
			nPrx++
		case e.Decision == reqlog.Direct:
			nDir++
		default:
			nUnknown++ // 解析不出决策（插件不可达等）
		}
	}

	ui.Column(c).Fill().Padding(sp4).Gap(sp3).Children(func() {
		// 视图切换（共用，保证能切回原始日志）
		a.logViewTabs(c)

		// 日期选择：实时 / 过去若干天（记录保留 7 天）
		a.requestDayRow(c)

		// 工具栏：搜索 + 仅失败胶囊 + 复制描边按钮 + ⋯ 菜单
		ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.logFilter).Grow(1).
				Placeholder(tr("搜索域名…", "Search host…"))
			chipToggle(c, &a.reqOnlyFailed, tr("仅看失败", "Failed"))
			ghostButton(c, tr("复制", "Copy"), func() {
				lines := make([]string, 0, len(entries))
				for _, e := range entries {
					lines = append(lines, e.At.Format("15:04:05")+"\t"+e.Target+"\t"+e.Decision.String())
				}
				c.Services().WriteClipboard(joinLines(lines))
			})
			menu := iconButton(c, "⋯")
			menu.Menu(func(m *ui.Menu) {
				if m.Item(tr("清空请求记录", "Clear requests")).Chosen() {
					a.reqs.Clear()
				}
			})
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
					// 底部汇总：当前列表的直连/代理/失败分布
					if len(entries) > 0 {
						ui.Row(c).FillWidth().Justify(ui.Center).Padding(sp2, 0).Children(func() {
							ui.Textf(c, "%s %d · %s %d · %s %d",
								tr("直连", "Direct"), nDir, tr("代理", "Proxy"), nPrx, tr("失败", "Failed"), nErr).
								FontSize(fsCaption).Font("monospace").TextColor(t.TextMuted)
							if nUnknown > 0 {
								ui.Textf(c, " · %s %d", tr("未知", "Unknown"), nUnknown).
									FontSize(fsCaption).Font("monospace").TextColor(t.TextMuted)
							}
						})
					}
				})
			})
		})
	})
}

// requestBadge 是去向胶囊：直连（灰）/ 代理（蓝淡底）/ 失败（红淡底）。
func requestBadge(c *ui.Context, label string, color ui.Color) {
	t := c.Theme()
	bg := t.SurfaceHover
	switch color {
	case t.Accent:
		bg = accentSoft(t)
	case t.Danger:
		bg = dangerSoft(t)
	}
	ui.Box(c).Width(44).Children(func() {
		ui.Box(c).Padding(2, sp2).Radius(5).Background(bg).
			Children(func() {
				ui.Text(c, label).FontSize(fsCaption).Bold().TextColor(color)
			})
	})
}

// requestRow 渲染一条请求：时间 · 去向胶囊 · 目标（下行是节点/错误）。
func (a *app) requestRow(c *ui.Context, e reqlog.Entry) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Padding(3, 2).Children(func() {
		ui.Text(c, e.At.Format("15:04:05")).FontSize(fsCaption).
			Font("monospace").TextColor(t.TextMuted).Width(58)

		// 去向：直连（灰）/ 代理（蓝）/ 失败（红）/ 未知（浅灰，插件调用失败时）
		//
		// 以前 Unknown 落到默认分支显示成“直连”，插件没跑时全部流量都
		// 看起来像走了直连，极具误导性。
		label, color := tr("未知", "Unknown"), t.TextMuted
		switch e.Decision {
		case reqlog.Direct:
			label, color = tr("直连", "Direct"), t.TextMuted
		case reqlog.Proxied:
			label, color = tr("代理", "Proxy"), t.Accent
		}
		if e.Err != "" {
			label, color = tr("失败", "Failed"), t.Danger
		}
		requestBadge(c, label, color)

		// 目标在上，节点/错误在下；目标缺失时（如只有 dial 错误）错误作主行
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			if e.Target != "" {
				ui.Text(c, e.Target).FontSize(fsBody).
					Font("monospace").SingleLine()
			}
			if e.Err != "" {
				main := e.Err != "" && e.Target == ""
				tx := ui.Text(c, e.Err).FontSize(fsCaption).SingleLine()
				if main {
					tx = tx.FontSize(fsBody).Font("monospace")
				} else {
					tx = tx.TextColor(t.Danger)
				}
			} else if e.Decision == reqlog.Proxied && e.Node != "" {
				ui.Textf(c, "→ %s", e.Node).FontSize(fsCaption).
					Font("monospace").TextColor(t.TextMuted).SingleLine()
			}
		})
	})
}

// requestCount 返回请求条数（用于日志页标签）。
func (a *app) requestCount() int { return a.reqs.Len() }

// formatLatency 把延迟格式化（当前未从日志采集，保留给后续扩展）。
func formatLatency(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Round(time.Millisecond).String()
}

var _ = proc.Running

// requestEntries 返回当前要显示的记录：实时取内存，历史取某天的落盘文件。
//
// 这里只读：解析在 internal/reqlog，落盘在 request_feed.go。
func (a *app) requestEntries() []reqlog.Entry {
	if a.reqDay == "" {
		return a.reqs.Entries(0)
	}
	st := a.reqs.Store()
	if st == nil {
		return nil
	}
	es, err := st.Load(a.reqDay)
	if err != nil {
		return nil
	}
	return es
}

// requestDayRow 是请求视图的日期切换行。
//
// 即使没有持久化（store 为 nil）也渲染：布局保持一致，
// 只是历史选项为空。
func (a *app) requestDayRow(c *ui.Context) {
	t := c.Theme()
	live := tr("实时", "Live")
	opts := []string{live}
	var st *reqlog.Store
	if a.reqs != nil {
		st = a.reqs.Store()
	}
	if st != nil {
		opts = append(opts, st.Days()...)
	}

	sel := live
	if a.reqDay != "" {
		sel = a.reqDay
	}
	if ui.Select(c, &sel, opts).Label(tr("查看", "View")).Changed() {
		if sel == live {
			a.reqDay = ""
		} else {
			a.reqDay = sel
		}
	}

	// 当天达到上限时提示（debug 日志可能很密）
	if st != nil && a.reqDay == "" {
		today := time.Now().Format("2006-01-02")
		if n := st.DayCount(today); n >= st.Limit() {
			ui.Textf(c, tr("今日记录已达上限 %d 条，后续请求不再保存",
				"Daily cap %d reached, further requests are not stored"), st.Limit()).
				FontSize(fsCaption).TextColor(t.Warning)
		}
	}
}
