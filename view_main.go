package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"gopkg.in/yaml.v3"
	"mygost/internal/geo"
	"mygost/internal/gostbin"
	"mygost/internal/metrics"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

// viewMain 主界面：按状态机分流——空应用只渲染引导，有配置渲染完整形态。
func (a *app) viewMain(c *ui.Context) {
	if len(a.ps.Items()) == 0 {
		a.viewEmpty(c)
		return
	}
	a.viewDashboard(c)
}

// viewEmpty 空状态：整页引导，单一中心点。
func (a *app) viewEmpty(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(sp3).Children(func() {
		statusDot(c, t.SurfaceHover, 56)
		ui.Text(c, "还没有配置").FontSize(fsDisplay).Bold()
		ui.Text(c, "添加本地代理或端口转发，一键连接").FontSize(fsBody).TextColor(t.TextMuted)
		ui.PrimaryButton(c, "添加配置").OnClick(a.startNew)
	})
}

// viewDashboard 有配置后的完整形态：状态条 + 大圆按钮 + 速率卡 + 配置列表。
func (a *app) viewDashboard(c *ui.Context) {
	t := c.Theme()
	state, started, _ := a.pm.State()
	running := state == proc.Running
	runningID := a.runningProfileID()
	current := a.st.Get().Current

	ui.Column(c).Fill().Children(func() {
		// ── 顶部状态条：品牌 + 连接状态（时长用等宽）──
		ui.Row(c).FillWidth().AlignItems(ui.Center).
			Padding(sp6, sp6, 0).Children(func() {
			ui.Text(c, "mygost").FontSize(fsEmphasis).Bold()
			ui.Box(c).Grow(1)
			if running {
				statusPill(c, t.Success, tr("已连接", "Connected"), formatDuration(time.Since(started)))
			} else {
				statusPill(c, mutedDot(t), tr("未连接", "Offline"))
			}
		})

		// ── 状态区：大圆按钮 + 当前配置 ──
		topPad := sp6
		if running {
			topPad = sp3 // 连接后收紧，给配置列表让出空间
		}
		ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(topPad, 0, 0).Children(func() {
			// 颜色 = 状态：灰(未就绪) / 蓝(可连接，主操作) / 绿(已连接)
			btnColor, btnText, btnTextColor := t.Surface, tr("连接", "Connect"), t.TextMuted
			switch {
			case running:
				btnColor, btnText, btnTextColor = t.Success, tr("断开", "Disconnect"), t.AccentText
			case current != "" && a.binFound:
				btnColor, btnTextColor = t.Accent, t.AccentText
			}
			// 双环光环：外环更淡一层，衰减感更强。
			// 尺寸必须显式给出，否则 Column 会把它横向拉满变成椭圆。
			d, ring1, ring2 := connectGeometry(running)
			outer := ui.Box(c).Width(ring2).Height(ring2).Radius(ring2 / 2).Center()
			if running {
				outer = outer.Background(successHaloOuter(t))
			}
			outer.Children(func() {
				inner := ui.Box(c).Width(ring1).Height(ring1).Radius(ring1 / 2).Center()
				switch {
				case running:
					inner = inner.Background(successHalo(t))
				case current != "" && a.binFound:
					inner = inner.Background(accentHalo(t))
				}
				inner.Children(func() {
					big := ui.ButtonBase(c).Width(d).Height(d).Radius(d / 2).Background(btnColor).
						Cursor(ui.CursorPointer).Children(func() {
						ui.Column(c).Fill().Center().Children(func() {
							ui.Text(c, btnText).FontSize(fsEmphasis).Bold().TextColor(btnTextColor)
						})
					})
					if big.Clicked() {
						go a.toggleMain(runningID)
					}
				})
			})

			// 当前配置名与摘要：让"连的是谁"一眼可见
			if p := a.currentProfile(); p != nil {
				ui.Column(c).AlignItems(ui.Center).Padding(sp3+2, 0, 0).Gap(1).Children(func() {
					ui.Text(c, p.Name).FontSize(fsEmphasis).Bold()
					ui.Text(c, p.Summary()).FontSize(fsCaption).
						Font("monospace").TextColor(t.TextMuted).SingleLine()
				})
			} else {
				ui.Text(c, tr("先在下方选用一个配置", "Pick a profile below")).
					FontSize(fsCaption).TextColor(t.TextMuted).Padding(sp3+2, 0, 0)
			}
		})

		// ── 速率卡：仅运行中且有数据时渲染（未连接不显示 0）──
		if running && a.metricsOK {
			ui.Box(c).Padding(sp6, sp6, 0).Children(func() {
				ui.Column(c).FillWidth().Radius(rCard).Background(t.Surface).
					Padding(sp3, 0, sp3-2).Children(func() {
					ui.Row(c).FillWidth().Children(func() {
						a.rateCell(c, tr("上传", "Up"), a.rates.Up, t.Accent)
						ui.Box(c).Width(1).Height(40).Background(t.Border)
						a.rateCell(c, tr("下载", "Down"), a.rates.Down, t.Success)
					})
					ui.Box(c).FillWidth().Height(1).Margin(sp3-2, sp4, 0).
						Background(t.Border).Opacity(0.6)
					ui.Row(c).FillWidth().Justify(ui.Center).Padding(sp3-2, 0, 0).Children(func() {
						ui.Textf(c, "%s　↑ %s　·　↓ %s", tr("本次会话", "This session"),
							metrics.FormatBytes(a.rates.TotalUp), metrics.FormatBytes(a.rates.TotalDown)).
							FontSize(fsCaption).Font("monospace").TextColor(t.TextMuted)
					})
				})
			})
		}

		// ── 分流状态卡：未连接时提醒用户分流在工作 ──
		if !running && a.st.Get().Split {
			ui.Column(c).FillWidth().Padding(sp6, sp6, 0).Gap(sp2).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(0, sp1).Children(func() {
					ui.Text(c, tr("分流", "Routing")).FontSize(fsCaption).Bold().TextColor(t.TextMuted).Grow(1)
					goSet := ui.ButtonBase(c).Padding(0, sp1).Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, tr("设置", "Settings")).FontSize(fsCaption).Bold().TextColor(t.Accent)
					})
					if goSet.Clicked() {
						a.page = "settings"
					}
				})
				goSettings := ui.ButtonBase(c).FillWidth().Padding(sp3, sp4).Radius(rCard).
					Background(t.Surface).Cursor(ui.CursorPointer).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp3).Children(func() {
						ui.Column(c).Grow(1).Gap(1).Children(func() {
							ui.Text(c, tr("国内直连", "Direct for CN")).FontSize(fsBody)
							ui.Textf(c, "%s · %s", tr("已开启", "On"), a.splitCountsDesc()).
								FontSize(fsCaption).TextColor(t.TextMuted)
						})
						miniSwitch(c, true)
					})
				})
				if goSettings.Clicked() {
					a.page = "settings"
				}
			})
		}

		// ── 配置列表：分节标题 + 分组卡片 ──
		// Grow(1)+Shrink(1)：占满剩余高度但不超过它，N 条配置只在卡内滚动，
		// 不会把底栏推出窗口（曾因只写 Shrink(1) 而被内容撞开）。
		ui.Column(c).FillWidth().Grow(1).Shrink(1).
			Padding(sp6, sp6, 0).Gap(sp2).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(0, sp1).Children(func() {
				// 带总数：列表超出可视区时，用户才知道还有几条（滚动条 hover 才显）
				ui.Textf(c, tr("配置 · %d", "Profiles · %d"), len(a.ps.Items())).
					FontSize(fsCaption).Bold().TextColor(t.TextMuted).Grow(1)
				add := ui.ButtonBase(c).Padding(0, sp1).Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, "＋ "+tr("添加", "Add")).FontSize(fsCaption).Bold().TextColor(t.Accent)
				})
				if add.Clicked() {
					a.startNew()
				}
			})
			ui.Box(c).FillWidth().Grow(1).Radius(rCard).
				Background(t.Surface).Clip().Children(func() {
				ui.Scroll(c).Fill().Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for i, p := range a.ps.Items() {
							if i > 0 {
								divider(c)
							}
							a.profileRow(c, p, p.ID == current, p.ID == runningID)
						}
					})
				})
			})
		})

		// ── gost 未安装横条：优先于列表展示 ──
		if !a.binFound {
			ui.Box(c).FillWidth().Padding(sp6, sp6, 0).Children(func() {
				ui.Box(c).FillWidth().Padding(sp3, sp4).Radius(rRow).
					Background(warningSoft(t)).Children(func() {
					ui.Row(c).Gap(sp3).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).Gap(1).Children(func() {
							ui.Text(c, "gost 未安装").FontSize(fsBody)
							ui.Text(c, "下载后才能连接").FontSize(fsCaption).TextColor(t.TextMuted)
						})
						if a.dlBusy {
							if a.dlTotal > 0 {
								ui.Textf(c, "%.0f%%", 100*float64(a.dlDone)/float64(a.dlTotal)).
									FontSize(fsCaption).TextColor(t.TextMuted)
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

		// ── 底部状态行：分流是否真的在工作 ──
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.Center).Gap(sp2).
			Padding(sp3, 0, sp3).Shrink(0).Children(func() {
			switch {
			case running && a.st.Get().Split:
				if a.geoM.Running() && portOpen(a.geoM.Addr()) {
					statusDot(c, t.Success, 7)
					ui.Text(c, tr("分流运行中 · 规则已就绪", "Routing active · rules ready")).
						FontSize(fsCaption).TextColor(t.TextMuted)
				} else {
					statusDot(c, t.Danger, 7)
					ui.Text(c, tr("插件未运行 · 将自动重启", "Plugin down · restarting")).
						FontSize(fsCaption).TextColor(t.Danger)
				}
			case !running:
				ui.Text(c, tr("连接后可查看实时速率与流量", "Rates and traffic appear once connected")).
					FontSize(fsCaption).TextColor(t.TextMuted)
			}
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

}

// rateCell 是速率卡的半边：标签 + 大号等宽数字。
func (a *app) rateCell(c *ui.Context, label string, v float64, color ui.Color) {
	t := c.Theme()
	ui.Column(c).Grow(1).AlignItems(ui.Center).Gap(1).Children(func() {
		ui.Text(c, label).FontSize(fsCaption).TextColor(t.TextMuted)
		ui.Text(c, metrics.FormatRate(v)).FontSize(fsStat).Bold().
			Font("monospace").TextColor(color)
	})
}

// miniSwitch 是纯展示的小开关（分流的"已开启"状态）。
func miniSwitch(c *ui.Context, on bool) {
	t := c.Theme()
	w, h, k := float32(36), float32(21), float32(17)
	bg := t.SurfaceHover
	if on {
		bg = t.Success
	}
	ui.Box(c).Width(w).Height(h).Radius(h / 2).Background(bg).Children(func() {
		// 白色旋钮在深浅两套主题里都成立
		knob := ui.Box(c).Width(k).Height(k).Radius(k / 2).Background(ui.RGB(255, 255, 255))
		if on {
			knob = knob.Margin(2, 0, 0, w-k-4)
		} else {
			knob = knob.Margin(2, 0, 0, 2)
		}
	})
}

// mutedDot 未连接状态的灰点。
func mutedDot(t *ui.Theme) ui.Color { return t.TextMuted }

// formatDuration 把会话时长格式化为 hh:mm:ss。
func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// profileRow 渲染分组卡片内的一行配置：点行 = 选用；编辑/删除在 ⋯ 菜单或右键。
func (a *app) profileRow(c *ui.Context, p *profiles.Profile, isCurrent, isRunning bool) {
	t := c.Theme()
	// 选中用淡底 + 左侧色条表达，不再用描边（描边只留给键盘焦点）
	bg := t.Surface
	if isCurrent {
		bg = accentSoft(t)
	}
	row := ui.ButtonBase(c).FillWidth().Padding(sp3, sp3).Radius(0).
		Background(bg).Cursor(ui.CursorPointer).
		Tooltip("点击选用；编辑在 ⋯ 菜单").Children(func() {
		ui.Row(c).FillWidth().Gap(sp3).AlignItems(ui.Center).Children(func() {
			// 左侧：选中 ✓ / 运行状态点（单一编码）
			if isCurrent {
				ui.Box(c).Width(3).Height(34).Radius(1.5).Background(t.Accent)
				ui.Box(c).Width(18).Height(18).Radius(9).Background(t.Accent).Center().
					Children(func() {
						ui.Text(c, "✓").FontSize(11).Bold().TextColor(t.AccentText)
					})
			} else {
				ui.Box(c).Width(3)
				ui.Box(c).Width(18).Height(18).Center().Children(func() {
					if isRunning {
						statusDot(c, t.Success, dotSm)
					}
				})
			}
			// 名称 + 类型标签 + 摘要（选中行有测试结果时，摘要行临时换成结果）
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Row(c).Gap(sp2).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).FontSize(fsEmphasis).Bold()
					tagPill(c, profiles.KindLabel(p.Kind), isCurrent)
				})
				if res, ok := a.testResultOf(p.ID); ok {
					a.inlineTestResult(c, res)
				} else {
					ui.Text(c, p.Summary()).FontSize(fsCaption).
						Font("monospace").TextColor(t.TextMuted).SingleLine()
				}
			})

			// 右侧：菜单
			menu := iconButton(c, "⋯")
			menu.Menu(func(m *ui.Menu) {
				if a.testing {
					m.Item(tr("测试中…", "Testing…")).Disabled(true)
				} else if m.Item(tr("测试连接", "Test connection")).Chosen() {
					go a.testProfile(p.ID)
				}
				m.Separator()
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
	})
	if row.Clicked() && !isCurrent {
		id := p.ID
		a.update(func(s *settings.Settings) { s.Current = id })
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(tr("测试连接", "Test connection")).Chosen() {
			go a.testProfile(p.ID)
		}
		m.Separator()
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

// connectGeometry 返回大圆按钮与双环光环的边长。
//
// 三者都必须显式给出尺寸：Column 会把未定宽的 Box 横向拉满，
// 光环就会变成横贯窗口的椭圆（v0.1 曾如此）。
//
// 已连接时收紧一档：此时速率与配置列表才是重点，按钮只需保持可点。
func connectGeometry(running bool) (button, ring1, ring2 float32) {
	if running {
		const d float32 = 84
		return d, d + 22, d + 50
	}
	const d float32 = 104
	return d, d + 28, d + 64
}

// toggleMain 主按钮：运行则断开，否则连接当前档。
func (a *app) toggleMain(runningID string) {
	if runningID != "" {
		a.stopGost()
		return
	}
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

	// 分流开启：先确保 geo 插件在运行（配置生成也会带上 bypasses）
	if s.Split {
		if err := a.ensureGeo(); err != nil {
			a.toast("分流不可用，按无分流启动: "+err.Error(), "error")
		}
	}

	// ① 已有同一个配置的 gost 在跑？直接接管（上次会话残留）
	if pid, started, ok := a.findOwn(id); ok {
		if err := a.pm.Attach(pid, started); err != nil {
			a.toast(tr("接管失败: ", "Attach failed: ")+err.Error(), "error")
			return
		}
		// 接管的进程也要挂上 metrics 抓取，否则已连接却看不到速率卡
		if port, ok := metricsPortFromYaml(a.ps.YamlPath(id)); ok {
			a.startMetrics(port)
		}
		a.toast(tr("已接管上次运行的 gost", "Attached to the running gost"), "success")
		return
	}

	// ② 端口被占用：先判断是不是我们自己的另一个配置
	if p := a.ps.Get(id); p != nil {
		if addr := probeAddr(p.Listen); addr != "" && portInUse(addr) {
			port := listenPort(p.Listen)
			ownerPid, ownerName := proc.PortOwner(port)
			if ownerName == "" && ownerPid == 0 {
				a.toast(tr("端口 ", "Port ")+p.Listen+tr(" 已被占用", " is in use"), "error")
				return
			}
			// 占用者是我们启动的 gost（命令行含配置目录）
			if isOurGost(ownerPid) {
				a.toast(tr("端口 ", "Port ")+p.Listen+
					tr(" 被本应用启动的另一个 gost 占用，请先断开它",
						" is held by another gost started by mygost; disconnect it first"), "error")
				return
			}
			who := ownerName
			if who == "" {
				who = fmt.Sprintf("pid %d", ownerPid)
			}
			a.toast(tr("端口 ", "Port ")+p.Listen+tr(" 被 ", " is held by ")+who+
				tr(" 占用", ""), "error")
			return
		}
	}

	// 选一个可用的 metrics 端口（用于流量统计），失败则不启用统计
	metricsPort, portErr := metrics.PickPort(9100, 10)
	metricsAddr := ""
	if portErr == nil {
		metricsAddr = "127.0.0.1:" + strconv.Itoa(metricsPort)
	}

	// 运行前按当前分流开关重写 yml（保存时的开关可能已变）
	if p := a.ps.Get(id); p != nil {
		sc := profiles.SplitConfig{
			Enabled: s.Split, PluginAddr: a.geoM.Addr(),
			PrivateMatchers: geo.PrivateMatchers, MetricsAddr: metricsAddr,
			// 请求决策只有 debug 级日志才有；设置里可关闭以省开销
			DebugLog: s.RequestLog,
		}
		if _, err := a.ps.SaveSplit(p, sc); err != nil {
			a.toast(tr("生成配置失败: ", "Failed to build config: ")+err.Error(), "error")
			return
		}
	}
	yml := a.ps.YamlPath(id)
	if err := a.pm.Start(bin, yml); err != nil {
		a.toast(tr("启动失败: ", "Failed: ")+err.Error(), "error")
		return
	}
	// 启动确认：gost 可能刚起来就因配置/端口问题退出，不能立刻报成功
	if err := a.pm.WaitStable(800 * time.Millisecond); err != nil {
		msg := err.Error()
		var se *proc.StartError
		if errors.As(err, &se) {
			msg = se.Detail()
		}
		a.toast(tr("启动失败: ", "Failed: ")+msg, "error")
		return
	}
	a.recordPid(id, bin, yml)
	if metricsAddr != "" {
		a.startMetrics(metricsPort)
	}
	a.toast(tr("已连接", "Connected"), "success")
}

// stateDir 存放 PID 等运行期状态。
func (a *app) stateDir() string { return filepath.Join(dataDir, "state") }

// recordPid 记录本次启动的进程，供下次会话识别/接管。
func (a *app) recordPid(profileID, bin, yml string) {
	pid := a.pm.Pid()
	if pid <= 0 {
		return
	}
	started, _, _ := startedAt(a.pm)
	rec := proc.Record{PID: pid, Profile: profileID, Config: yml, Binary: bin, Started: started}
	if err := proc.WritePIDFile(a.stateDir(), rec); err != nil {
		log.Printf("写 pid 文件失败: %v", err)
	}
}

// clearPid 在停止后清理 PID 文件。
func (a *app) clearPid() { proc.RemovePIDFile(a.stateDir()) }

// findOwn 查找属于本应用的、指定配置的 gost 进程。
// 先看 PID 文件（权威），再用命令行扫描兜底（覆盖旧版本遗留）。
func (a *app) findOwn(profileID string) (pid int, started time.Time, ok bool) {
	yml := a.ps.YamlPath(profileID)

	if rec, valid := proc.ReadPIDFile(a.stateDir()); valid && rec.Config == yml {
		return rec.PID, rec.Started, true
	}
	// 兜底：命令行里带这个配置文件的进程
	for _, p := range proc.FindByConfig(yml) {
		return p, time.Time{}, true
	}
	return 0, time.Time{}, false
}

// isOurGost 判断 pid 是否为 mygost 启动的 gost（命令行含 profiles 目录）。
func isOurGost(pid int) bool {
	if pid <= 0 {
		return false
	}
	return proc.CommandlineContains(pid, filepath.Join(dataDir, "profiles"))
}

// startedAt 返回进程启动时间（接管场景下可能为零值）。
func startedAt(pm *proc.Manager) (time.Time, bool, bool) {
	st, started, reason := pm.State()
	return started, st == proc.Running, reason == ""
}

// listenPort 从监听地址里取出端口号。
func listenPort(listen string) int {
	addr := probeAddr(listen)
	if addr == "" {
		return 0
	}
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, _ := strconv.Atoi(portStr)
	return p
}

// probeAddr 把监听地址转成可探测的 host:port。
// ":8026" 这类只给端口的写法按本机回环探测。
func probeAddr(listen string) string {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return ""
	}
	if strings.HasPrefix(listen, ":") {
		return "127.0.0.1" + listen
	}
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return ""
	}
	return listen
}

// portInUse 报告 addr 是否已被监听。
func portInUse(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err == nil {
		c.Close()
		return true
	}
	return false
}

// stopGost 统一停止入口：停进程并清理 PID 记录。
func (a *app) stopGost() {
	if err := a.pm.Stop(); err != nil {
		a.toast(tr("停止失败: ", "Stop failed: ")+err.Error(), "error")
		return
	}
	a.clearPid()
	a.stopMetrics()
	// 插件跟随 gost：gost 停了就不再需要它
	a.geoM.Stop()
}

// watchSplit 守护插件与 gost 的绑定关系：
// gost 在运行且分流开启时，插件必须在运行；不在就自动补启。
//
// 这样无论插件是被 app 退出、崩溃还是被手动 kill 掉的，都会自愈。
func (a *app) watchSplit() {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			if a.win == nil || a.win.IsDestroyed() {
				return
			}
			if st, _, _ := a.pm.State(); st != proc.Running {
				continue
			}
			if !a.st.Get().Split {
				continue
			}
			if a.geoM.Running() && portOpen(a.geoM.Addr()) {
				continue
			}
			log.Printf("分流插件未运行，自动补启")
			if err := a.ensureGeo(); err != nil {
				log.Printf("补启分流插件失败: %v", err)
			}
			a.win.Update(func() { a.refreshTray() })
		}
	}()
}

// portOpen 报告 addr 是否可连（判断插件是否真的在工作）。
func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// ensureGeo 确保分流插件运行且规则就绪；规则缺失时自动下载。
func (a *app) ensureGeo() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	st := a.st.Get()
	if a.geoM.CountIP() <= 0 {
		if _, err := a.geoM.UpdateIPList(ctx, st.SplitIPListURL); err != nil {
			return fmt.Errorf("更新 IP 列表失败: %w", err)
		}
	}
	if a.geoM.CountDomains() <= 0 {
		if _, err := a.geoM.UpdateDomainList(ctx, st.SplitDomainListURL); err != nil {
			return fmt.Errorf("更新域名列表失败: %w", err)
		}
	}
	if a.geoM.Running() {
		return nil // 已在运行；规则文件热加载由 gost reload 处理
	}
	if err := a.geoM.Start(ctx); err != nil {
		return err
	}
	return a.geoM.WaitReady(ctx, 10*time.Second)
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
	total := int(d.Seconds()) // 注意：int(d) 是纳秒，必须用 Seconds()
	h := total / 3600
	m := total % 3600 / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// metricsPortFromYaml 从已生成的配置里读 metrics 端口（接管路径用：
// 接管的 gost 配置里已写好 metrics.addr，解析出来即可挂上抓取）。
func metricsPortFromYaml(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var doc struct {
		Metrics struct {
			Addr string `yaml:"addr"`
		} `yaml:"metrics"`
	}
	if yaml.Unmarshal(b, &doc) != nil {
		return 0, false
	}
	_, port, err := net.SplitHostPort(doc.Metrics.Addr)
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
