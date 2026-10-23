package main

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/gostbin"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

// viewMain 主界面：大连接按钮（中心点）+ 配置列表 + 底部工具栏。
func (a *app) viewMain(c *ui.Context) {
	t := c.Theme()
	state, started, _ := a.pm.State()
	running := state == proc.Running
	runningID := a.runningProfileID()
	current := a.st.Get().Current

	ui.Column(c).Fill().Children(func() {
		// ── 中心点：连接状态与大圆按钮 ──
		ui.Column(c).Fill().Center().Gap(10).Padding(20, 0, 8).Children(func() {
			btnColor := t.Surface
			btnText := "连接"
			btnTextColor := t.TextMuted
			switch {
			case running:
				btnColor, btnText, btnTextColor = t.Danger, "断开", t.AccentText
			case current != "" && a.binFound:
				btnColor, btnTextColor = t.Success, t.AccentText
			}
			// 大圆连接按钮（无外观按钮 + 自绘样式，支持键盘与无障碍）
			big := ui.ButtonBase(c).Width(76).Height(76).Radius(38).Background(btnColor).
				Cursor(ui.CursorPointer).Children(func() {
				ui.Column(c).Fill().Center().Children(func() {
					ui.Text(c, btnText).Bold().TextColor(btnTextColor)
				})
			})
			if big.Clicked() {
				go a.toggleMain(runningID)
			}
			// 状态行
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				dot := t.TextMuted
				if running {
					dot = t.Success
				} else if state == proc.Crashed {
					dot = t.Danger
				}
				statusDot(c, dot, 8)
				label := state.String()
				if running {
					label = fmt.Sprintf("%s · %s", state.String(), fmtDuration(time.Since(started)))
				}
				ui.Text(c, label).FontSize(15)
			})
			if p := a.currentProfile(); p != nil {
				ui.Text(c, p.Name).FontSize(13).TextColor(t.TextMuted)
			}
		})

		// ── gost 未安装横条 ──
		if !a.binFound {
			ui.Box(c).Padding(0, 16).Children(func() {
				ui.Box(c).Fill().Padding(10, 14).Radius(10).Border(1, t.Warning).Children(func() {
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "gost 未安装").Grow(1)
						if a.dlBusy {
							if a.dlTotal > 0 {
								ui.Textf(c, "下载中 %.0f%%", 100*float64(a.dlDone)/float64(a.dlTotal)).TextColor(t.TextMuted)
							} else {
								ui.Spinner(c)
							}
						} else {
							ui.PrimaryButton(c, "下载 "+gostbin.DefaultVersion).OnClick(func() { go a.doDownload() })
						}
					})
				})
			})
		}

		// ── 配置列表标题 ──
		ui.Row(c).AlignItems(ui.Center).Padding(14, 16, 6).Children(func() {
			ui.Text(c, "配置").FontSize(13).Bold().TextColor(t.TextMuted).Grow(1)
			ui.Button(c, "+ 添加").OnClick(a.startNew)
		})

		// ── 配置列表 ──
		ui.Box(c).Fill().Grow(1).Padding(0, 16).Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).Fill().Gap(8).Children(func() {
					items := a.ps.Items()
					if len(items) == 0 {
						ui.Column(c).Fill().Center().Gap(8).Padding(28).Children(func() {
							ui.Text(c, "还没有配置").Bold()
							ui.Text(c, "添加一个本地代理或端口转发。").TextColor(t.TextMuted)
						})
						return
					}
					for _, p := range items {
						a.profileRow(c, p, p.ID == current, p.ID == runningID)
					}
				})
			})
		})

		// ── 底部工具栏 ──
		ui.Row(c).Fill().Padding(6, 16).Gap(8).Children(func() {
			ui.Button(c, "日志").Grow(1).OnClick(func() { a.page = "logs" })
			ui.Button(c, "设置").Grow(1).OnClick(func() { a.page = "settings" })
		})
	})

	// 删除确认（每帧至多一个，挂在主界面层）
	if a.delOpen {
		if r := ui.AlertDialog(c, &a.delOpen, "删除配置", "确定删除「"+a.delTitle+"」吗？", "取消", "删除"); r == 1 {
			id := a.delTarget
			if err := a.ps.Delete(id); err != nil {
				a.toast(err.Error(), "error")
			} else {
				a.update(func(s *settings.Settings) {
					if s.Current == id {
						s.Current = ""
					}
				})
				a.toast("已删除", "success")
			}
		}
	}

	// 运行中每秒刷新时长
	if running {
		c.After(time.Second)
	}
}

// profileRow 渲染一行配置：状态点 + 名称摘要 + 连接控件 + 菜单。
func (a *app) profileRow(c *ui.Context, p *profiles.Profile, isCurrent, isRunning bool) {
	t := c.Theme()
	ui.Box(c).Fill().Padding(12, 14).Radius(10).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			// 状态点：运行绿 / 当前蓝 / 其他灰
			dot := t.Border
			if isRunning {
				dot = t.Success
			} else if isCurrent {
				dot = t.Accent
			}
			statusDot(c, dot, 8)

			// 名称与摘要
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).FontSize(15)
					ui.Badge(c, profiles.KindLabel(p.Kind))
				})
				ui.Text(c, p.Summary()).FontSize(12).TextColor(t.TextMuted)
			})

			// 连接控件：互斥，一次只启动一个
			if isRunning {
				ui.Text(c, "已连接").FontSize(13).Bold().TextColor(t.Success)
				ui.Button(c, "断开").OnClick(func() { go a.pm.Stop() })
			} else {
				btn := ui.Button(c, "连接")
				if !a.binFound {
					btn = btn.Disabled(true)
				}
				btn.OnClick(func() { go a.connectProfile(p.ID) })
			}

			// 行菜单：编辑 / 删除（点击 ⋯ 或右键均可）
			ui.Text(c, "⋯").TextColor(t.TextMuted).Menu(func(m *ui.Menu) {
				if m.Item("编辑").Chosen() {
					a.startEdit(p)
				}
				if m.Item("删除").Chosen() {
					a.delTarget = p.ID
					a.delTitle = p.Name
					a.delOpen = true
				}
			})
		})
	}).ContextMenu(func(m *ui.Menu) {
		if m.Item("编辑").Chosen() {
			a.startEdit(p)
		}
		if m.Item("删除").Chosen() {
			a.delTarget = p.ID
			a.delTitle = p.Name
			a.delOpen = true
		}
	})
}

// toggleMain 主按钮：运行则断开，否则连接当前档。
func (a *app) toggleMain(runningID string) {
	if runningID != "" {
		if err := a.pm.Stop(); err != nil {
			a.toast("停止失败: "+err.Error(), "error")
		}
		return
	}
	a.doStart()
}

// connectProfile 连接指定档；若有档在运行，先停止旧的（互斥）。
func (a *app) connectProfile(id string) {
	if runningID := a.runningProfileID(); runningID != "" {
		a.pm.StopAndWait(5 * time.Second)
	}
	a.update(func(s *settings.Settings) { s.Current = id })
	a.doStart()
}

// currentProfile 返回当前选中档（无则 nil）。
func (a *app) currentProfile() *profiles.Profile {
	s := a.st.Get()
	if s.Current == "" {
		return nil
	}
	return a.ps.Get(s.Current)
}

// doStart 找二进制并启动当前档（后台 goroutine 执行）。
func (a *app) doStart() {
	s := a.st.Get()
	id := s.Current
	if id == "" {
		a.toast("请先选择一个配置", "error")
		return
	}
	bin, _, err := a.loc.Find(s.GostPath)
	if err != nil {
		a.win.Update(func() { a.binFound = false })
		a.toast("未找到 gost，请先下载", "error")
		return
	}
	yml := a.ps.YamlPath(id)
	if err := a.pm.Start(bin, yml); err != nil {
		a.toast("启动失败: "+err.Error(), "error")
		return
	}
	a.toast("已连接", "success")
}

// doDownload 后台下载 gost。
func (a *app) doDownload() {
	a.win.Update(func() { a.dlBusy = true; a.dlDone, a.dlTotal = 0, 0 })
	_, err := a.loc.Download(context.Background(), a.st.Get().Mirror, func(done, total int64) {
		a.win.Update(func() { a.dlDone, a.dlTotal = done, total })
	})
	a.win.Update(func() {
		a.dlBusy = false
		if err != nil {
			a.pendingToasts = append(a.pendingToasts, ui.Toast{Title: "下载失败: " + err.Error(), Type: "error"})
			return
		}
		a.pendingToasts = append(a.pendingToasts, ui.Toast{Title: "下载完成", Type: "success"})
		go a.checkBinAsync()
	})
}

// checkBinAsync 后台检测二进制与版本。
func (a *app) checkBinAsync() {
	go func() {
		s := a.st.Get()
		path, src, err := a.loc.Find(s.GostPath)
		ver := ""
		if err == nil {
			ver = gostbin.Version(path)
		}
		found := err == nil
		a.win.Update(func() {
			a.binFound = found
			a.binPath, a.binSource, a.binVer = path, src, ver
		})
	}()
}

func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d) / 3600
	m := int(d) % 3600 / 60
	s := int(d) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
