package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
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

// viewDashboard 有配置后的完整形态：连接按钮 + gost 横条 + 配置列表。
func (a *app) viewDashboard(c *ui.Context) {
	t := c.Theme()
	state, _, _ := a.pm.State()
	running := state == proc.Running
	runningID := a.runningProfileID()
	current := a.st.Get().Current

	ui.Column(c).Fill().Children(func() {
		// ── 上 3：状态区（大圆按钮 = 连接当前选中档）──
		ui.Column(c).FillWidth().HeightPercent(30).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			// 颜色 = 状态：灰(未就绪) / 蓝(可连接，主操作) / 绿(已连接)
			// 文字 = 动作：连接 / 断开。已连接时外加光环，让绿色只声明状态。
			btnColor, btnText, btnTextColor := t.Surface, "连接", t.TextMuted
			switch {
			case running:
				btnColor, btnText, btnTextColor = t.Success, "断开", t.AccentText
			case current != "" && a.binFound:
				btnColor, btnTextColor = t.Accent, t.AccentText
			}
			// 光环与按钮同心：用等尺寸的 Box 包一层，尺寸必须显式给出，
			// 否则 Column 会把它横向拉满，光环变成横贯窗口的椭圆。
			d, ringSize := connectGeometry()
			halo := ui.Box(c).Width(ringSize).Height(ringSize).
				Radius(ringSize / 2).Center()
			if running {
				halo = halo.Background(successHalo(t))
			}
			halo.Children(func() {
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

			// 实时速率：仅在运行中且有数据时渲染（未连接时不显示 0）
			if running && a.metricsOK {
				ui.Row(c).Gap(sp3).Children(func() {
					ui.Text(c, "↑ "+metrics.FormatRate(a.rates.Up)).
						FontSize(fsCaption).TextColor(t.TextMuted)
					ui.Text(c, "↓ "+metrics.FormatRate(a.rates.Down)).
						FontSize(fsCaption).TextColor(t.TextMuted)
				})
			}
		})

		// ── gost 未安装横条 ──
		if !a.binFound {
			ui.Box(c).Padding(0, sp4).Children(func() {
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

		// ── 下 7：配置列表 ──
		ui.Box(c).Fill().Grow(1).Padding(sp3, sp4, sp4).Children(func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					for _, p := range a.ps.Items() {
						a.profileRow(c, p, p.ID == current, p.ID == runningID)
					}
					// 列表末尾：轻量添加入口（次级操作，不与配置卡片抢视觉）
					add := ui.ButtonBase(c).FillWidth().Height(36).Radius(rRow).
						Cursor(ui.CursorPointer).Children(func() {
						ui.Text(c, "＋ 添加配置").FontSize(fsBody).TextColor(t.TextMuted)
					})
					if add.Clicked() {
						a.startNew()
					}
				})
			})
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

// profileRow 渲染一行配置：点行 = 选用；编辑/删除在 ⋯ 菜单或右键。
func (a *app) profileRow(c *ui.Context, p *profiles.Profile, isCurrent, isRunning bool) {
	t := c.Theme()
	// 选中用淡底 + 左侧色条表达，不再用描边（描边只留给键盘焦点）
	bg := t.Surface
	if isCurrent {
		bg = accentSoft(t)
	}
	row := ui.ButtonBase(c).FillWidth().Padding(sp3, sp3).Radius(rRow).
		Background(bg).Cursor(ui.CursorPointer).
		Tooltip("点击选用；编辑在 ⋯ 菜单").Children(func() {
		ui.Row(c).FillWidth().Gap(sp3).AlignItems(ui.Start).Children(func() {
			// 左侧：选中标记 / 运行状态点（单一编码）
			if isCurrent {
				ui.Box(c).Width(3).Height(34).Radius(1.5).Background(t.Accent)
			}
			// 名称与摘要（选中行有测试结果时，摘要行临时换成结果）
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Row(c).Gap(sp2).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).FontSize(fsEmphasis)
					if isRunning {
						statusDot(c, t.Success, dotSm)
					}
					ui.Text(c, profiles.KindLabel(p.Kind)).FontSize(fsCaption).TextColor(t.TextMuted)
				})
				if res, ok := a.testResultOf(p.ID); ok {
					a.inlineTestResult(c, res)
				} else {
					ui.Text(c, p.Summary()).FontSize(fsBody).TextColor(t.TextMuted).SingleLine()
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

// connectGeometry 返回大圆按钮与光环的边长。
//
// 两者都必须显式给出尺寸：Column 会把未定宽的 Box 横向拉满，
// 光环就会变成横贯窗口的椭圆（v0.1 曾如此）。
func connectGeometry() (button, halo float32) {
	const d float32 = 88
	return d, d + sp3*2
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
}

// ensureGeo 确保分流插件运行且规则就绪；规则缺失时自动下载。
func (a *app) ensureGeo() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if !a.geoM.ListReady() {
		if _, err := a.geoM.UpdateIPList(ctx, a.st.Get().SplitIPListURL); err != nil {
			return fmt.Errorf("更新 IP 列表失败: %w", err)
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
