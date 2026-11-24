package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mygost/internal/geo"
	"mygost/internal/metrics"
	"mygost/internal/proc"
	"mygost/internal/settings"
)

// viewSettings 设置页：改动即时生效，改动后有短时"已保存"反馈。
func (a *app) viewSettings(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(sp4).Gap(sp3).Children(func() {
			// ── 分流 ──
			sectionHeader(c, tr("分流", "Routing"))
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					settingRow(c, tr("国内直连", "Direct for CN"),
						tr("国内域名与 IP 直接连接，其余走隧道",
							"CN domains and IPs connect directly, others via tunnel"), func() {
							if ui.Switch(c, &a.sSplit).Changed() {
								a.onSplitToggled()
							}
						})
					if a.sSplit {
						ui.Divider(c)
						// 插件状态持久可见：分流是否真的在工作
						a.splitStatusRow(c)
						ui.Divider(c)
						settingRow(c, tr("域名列表", "Domains"), a.domainListDesc(), func() {
							if a.splitBusy.Load() {
								ui.Spinner(c)
							} else {
								smallButton(c, tr("更新", "Update"), func() { go a.updateSplitLists() })
							}
						})
						ui.Divider(c)
						settingRow(c, tr("IP 列表", "IP list"), a.ipListDesc(), func() {
							if a.splitBusy.Load() {
								ui.Spinner(c)
							} else {
								smallButton(c, tr("更新", "Update"), func() { go a.updateSplitLists() })
							}
						})
						ui.Divider(c)
						settingRow(c, tr("未收录域名", "Unknown domains"),
							tr("不在列表中的域名如何处理", "How to handle unlisted domains"), func() {
								if segmentedFixed(c, &a.sSplitPolicy, formCtlW,
									tr("走代理", "Proxy"), tr("按 IP 判断", "By IP")) {
									a.saveSettings()
									a.geoM.SetPolicy(policyOf(a.sSplitPolicy))
									a.reconnectIfRunning()
								}
							})
					}
				})
			})

			// ── 行为 ──
			sectionHeader(c, tr("行为", "Behavior"))
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					settingRow(c, "记录请求明细", "记录每个请求是直连还是走代理（debug 日志，量较大）", func() {
						if ui.Switch(c, &a.sReqLog).Changed() {
							a.saveSettings()
							a.reqs.SetEnabled(a.sReqLog)
							a.toast(tr("重新连接后生效", "Takes effect on next connect"), "success")
						}
					})
					divider(c)
					settingRow(c, "退出时保持运行", "关闭 mygost 后 gost 继续工作", func() {
						if ui.Switch(c, &a.sKeep).Changed() {
							a.saveSettings()
						}
					})
					divider(c)
					settingRow(c, "开机自启", "登录后自动运行 mygost", func() {
						if ui.Switch(c, &a.sLogin).Changed() {
							if err := mygo.App.SetOpenAtLogin(a.sLogin); err != nil {
								a.toast("设置失败: "+err.Error(), "error")
							}
						}
					})
					divider(c)
					settingRow(c, "自动运行", "启动后连接当前配置", func() {
						if ui.Switch(c, &a.sAuto).Changed() {
							a.saveSettings()
						}
					})
				})
			})

			// ── gost 程序 ──
			sectionHeader(c, "GOST 程序")
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {

					settingField(c, "本地路径", "留空时自动查找：数据目录 → 系统 PATH", func() {
						ui.Row(c).FillWidth().Gap(sp2).Children(func() {
							ui.TextInput(c, &a.sGostPath).Grow(1).
								Placeholder("/usr/local/bin/gost").
								OnChange(a.saveSettingsDebounced)
							ui.Button(c, "浏览…").OnClick(a.browseGost)
						})
					})

					divider(c)

					settingField(c, "下载镜像", "可用变量：{version} {os} {arch} {ext}", func() {
						ui.Row(c).FillWidth().Gap(sp2).Children(func() {
							ui.TextInput(c, &a.sMirror).Grow(1).
								Placeholder("默认 GitHub Releases").
								OnChange(a.saveSettingsDebounced)
							ui.Button(c, "默认").OnClick(func() {
								a.sMirror = ""
								a.saveAndCheck()
							})
						})
					})

					divider(c)

					settingRow(c, "当前版本", versionLabel(a), func() {
						if a.dlBusy {
							ui.Spinner(c)
						} else {
							smallButton(c, tr("下载 / 更新", "Download"), func() { go a.doDownload() })
						}
					})
				})
			})

			// ── 维护 ──
			sectionHeader(c, tr("维护", "Maintenance"))
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					settingRow(c, tr("残留进程", "Leftover processes"),
						a.leftoverDesc(), func() {
							if len(a.vm.leftovers) > 0 {
								ui.Button(c, tr("全部结束", "Kill all")).OnClick(func() { go a.killLeftovers() })
							}
						})
				})
			})

			// ── 关于 ──
			sectionHeader(c, tr("关于", "About"))
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					kvRow(c, tr("应用", "App"), "mygost "+appVersion+" · "+tr("gost 桌面客户端", "gost desktop client"))
					// 数据目录：显示实际路径与来源，可打开/更改/恢复默认
					a.dataDirRow(c)
					// 日志：Windows 上双击出问题只能靠它排查，做成可点击打开
					settingRow(c, tr("运行日志", "Log file"), a.logPath, func() {
						if a.logPath != "" {
							ui.Button(c, tr("打开", "Open")).OnClick(func() {
								mygo.Shell.OpenPath(filepath.Dir(a.logPath))
							})
						}
					})
					if a.stats != nil {
						tod, tot := a.stats.Today(), a.stats.Total()
						kvRow(c, "今日流量", metrics.FormatBytes(tod.Up)+" ↑ / "+metrics.FormatBytes(tod.Down)+" ↓")
						kvRow(c, "近 31 天", metrics.FormatBytes(tot.Up)+" ↑ / "+metrics.FormatBytes(tot.Down)+" ↓")
					}
					a.dataDirPendingBlock(c)
				})
			})

			// 保存反馈：短时提示，不打扰
			if time.Now().Before(a.savedUntil) {
				ui.Text(c, "已保存").FontSize(fsCaption).TextColor(t.Success)
				c.After(a.savedUntil.Sub(time.Now()))
			}
		})
	})
}

// versionLabel 描述当前 gost 二进制状态。
func versionLabel(a *app) string {
	switch {
	case a.dlBusy:
		return "正在下载…"
	case a.binVer != "":
		return a.binVer
	case a.binFound:
		return "已就绪"
	default:
		return "未安装"
	}
}

// browseGost 打开文件选择框。
func (a *app) browseGost() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择 gost 程序"})
	if err != nil || len(paths) == 0 {
		return
	}
	a.win.Update(func() {
		a.sGostPath = paths[0]
		a.saveAndCheck()
	})
}

// saveSettings 即时落盘设置，并显示短时反馈。
func (a *app) saveSettings() {
	if a.saveTimer != nil {
		a.saveTimer.Stop()
		a.saveTimer = nil
	}
	a.update(func(s *settings.Settings) {
		s.GostPath = a.sGostPath
		s.Mirror = a.sMirror
		s.KeepOnExit = a.sKeep
		s.AutoStart = a.sAuto
		s.Split = a.sSplit
		s.SplitIPListURL = a.sSplitURL
		s.SplitPolicy = "proxy"
		if a.sSplitPolicy == 1 {
			s.SplitPolicy = "resolve"
		}
		s.RequestLog = a.sReqLog
	})
	a.savedUntil = time.Now().Add(2 * time.Second)
}

// saveSettingsDebounced 文本输入防抖：停止输入 600ms 后才落盘。
func (a *app) saveSettingsDebounced() {
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(600*time.Millisecond, func() {
		a.applyUI(a.saveSettings)
	})
}

// saveAndCheck 落盘并重新检测 gost 二进制（路径/镜像变化时用）。
func (a *app) saveAndCheck() {
	a.saveSettings()
	a.checkBinAsync()
}

// listDesc 描述一份规则列表的状态。
func (a *app) listDesc(count int, at time.Time) string {
	if a.splitBusy.Load() {
		return tr("更新中…", "Updating…")
	}
	if count < 0 {
		return tr("尚未下载", "Not downloaded")
	}
	if at.IsZero() {
		return fmt.Sprintf(tr("%d 条", "%d entries"), count)
	}
	return fmt.Sprintf("%d 条 · %s", count, at.Format("2006-01-02 15:04"))
}

// ipListDesc 是 IP 列表行的描述文字（行数来自后台探测的 vm，不读文件）。
func (a *app) ipListDesc() string {
	return a.listDesc(a.vm.ipCount, a.vm.ipAt)
}

// domainListDesc 是域名列表行的描述文字（行数来自 vm）。
func (a *app) domainListDesc() string {
	return a.listDesc(a.vm.domainCount, a.vm.domainAt)
}

// onSplitToggled 在开关变化时落盘；开启即预下载规则，保证下次连接可用。
//
// 分流开关只在生成配置时生效，所以运行中改动必须重连才有意义：
// 这里自动重连，避免用户以为设置没起作用。
func (a *app) onSplitToggled() {
	a.saveSettings()
	if a.sSplit && !a.geoM.Ready() && !a.splitBusy.Load() {
		go a.updateSplitLists()
	}
	a.reconnectIfRunning()
}

// reconnectIfRunning 在 gost 运行中时重连，使新配置生效。
func (a *app) reconnectIfRunning() {
	if st, _, _ := a.pm.State(); st != proc.Running {
		return
	}
	a.toast(tr("正在重新连接以应用设置", "Reconnecting to apply settings"), "success")
	go func() {
		a.stopGost()
		time.Sleep(400 * time.Millisecond)
		a.doStart()
	}()
}

// updateSplitLists 在后台更新两份列表（IP 与域名）。
func (a *app) updateSplitLists() {
	if !a.splitBusy.CompareAndSwap(false, true) {
		return
	}
	a.applyUI(func() {})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st := a.st.Get()

	ips, ipErr := a.geoM.UpdateIPList(ctx, st.SplitIPListURL)
	domains, domErr := a.geoM.UpdateDomainList(ctx, st.SplitDomainListURL)

	a.applyUI(func() {
		a.splitBusy.Store(false)
		switch {
		case ipErr != nil:
			a.pendingToasts = append(a.pendingToasts,
				ui.Toast{Title: tr("IP 列表更新失败: ", "IP list: ") + ipErr.Error(), Type: "error"})
		case domErr != nil:
			a.pendingToasts = append(a.pendingToasts,
				ui.Toast{Title: tr("域名列表更新失败: ", "Domain list: ") + domErr.Error(), Type: "error"})
		default:
			a.pendingToasts = append(a.pendingToasts, ui.Toast{
				Title: fmt.Sprintf(tr("已更新 %d 条 IP、%d 个域名", "Updated %d IPs, %d domains"), ips, domains),
				Type:  "success",
			})
		}
	})
}

// leftovers 返回不属于当前会话、但由本应用启动的 gost 进程。
//
// 这是实时扫描（用户点“全部结束”时用），不要放进渲染或周期探测路径。
func (a *app) leftovers() []int {
	all := proc.FindByConfig(filepath.Join(dataDir, "profiles"), a.gostExeDirs()...)
	cur := a.pm.Pid()
	out := make([]int, 0, len(all))
	for _, p := range all {
		if p != cur {
			out = append(out, p)
		}
	}
	return out
}

// leftoverScanInterval 是残留进程扫描的最小间隔。
//
// pgrep 要遍历整张进程表（macOS 上实测约 40ms）。之前每 2 秒探测一次
// 等于常驻占用约 2% 单核；残留进程变化很慢，15 秒足够。
const leftoverScanInterval = 15 * time.Second

// viewLeftovers 返回供界面展示的残留进程（带缓存，渲染/探测路径用）。
func (a *app) viewLeftovers() []int {
	a.leftoverMu.Lock()
	defer a.leftoverMu.Unlock()
	if a.leftoverOK && time.Since(a.leftoverAt) < leftoverScanInterval {
		return a.leftoverPids
	}
	pids := a.leftovers()
	a.leftoverAt, a.leftoverPids, a.leftoverOK = time.Now(), pids, true
	return pids
}

// invalidateLeftovers 让下一次探测重新扫描（结束进程后调用）。
func (a *app) invalidateLeftovers() {
	a.leftoverMu.Lock()
	a.leftoverOK = false
	a.leftoverMu.Unlock()
}

// leftoverDesc 描述残留进程数量（数量来自后台探测的 vm）。
func (a *app) leftoverDesc() string {
	n := len(a.vm.leftovers)
	if n == 0 {
		return tr("没有残留进程", "None")
	}
	return fmt.Sprintf(tr("%d 个进程未被本会话管理", "%d process(es) not managed by this session"), n)
}

// killLeftovers 结束所有残留的 gost 进程。
func (a *app) killLeftovers() {
	n := 0
	for _, pid := range a.leftovers() {
		if err := proc.Terminate(pid); err == nil {
			n++
		}
	}
	a.invalidateLeftovers()
	a.win.Update(func() {
		if n == 0 {
			a.pendingToasts = append(a.pendingToasts,
				ui.Toast{Title: tr("没有可结束的进程", "Nothing to kill"), Type: "success"})
			return
		}
		a.pendingToasts = append(a.pendingToasts,
			ui.Toast{Title: fmt.Sprintf(tr("已结束 %d 个进程", "Killed %d process(es)"), n), Type: "success"})
	})
}

// policyOf 把界面索引转成插件策略。
func policyOf(idx int) geo.Policy {
	if idx == 1 {
		return geo.PolicyResolve
	}
	return geo.PolicyProxy
}

// splitStatusRow 显示分流插件的实时状态。
func (a *app) splitStatusRow(c *ui.Context) {
	t := c.Theme()
	runningGost := false
	if st, _, _ := a.pm.State(); st == proc.Running {
		runningGost = true
	}
	alive := a.vm.splitAlive

	dot, text := t.TextMuted, tr("未运行", "Not running")
	sub := ""
	switch {
	case alive:
		dot, text = t.Success, tr("插件运行中", "Plugin running")
		sub = fmt.Sprintf("%s · %s", a.vm.splitAddr, a.splitCountsDesc())
	case runningGost:
		// gost 在跑但插件不在 = 分流实际未生效，必须警告
		dot, text = t.Danger, tr("插件未运行，分流未生效",
			"Plugin down — routing inactive")
		// 显示真实原因（端口占用/规则缺失/启动失败），不猜
		if a.vm.splitErr != "" {
			sub = a.vm.splitErr
		} else {
			sub = tr("正在重启…", "Restarting…")
		}
	case a.vm.rulesReady:
		dot, text = t.TextMuted, tr("连接后启动", "Starts with connection")
	default:
		dot, text = t.Warning, tr("规则未下载", "Rules not downloaded")
	}

	ui.Row(c).AlignItems(ui.Center).Gap(sp3).Padding(sp1, 0).Children(func() {
		statusDot(c, dot, dotSm)
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, text).FontSize(fsBody).TextColor(t.TextMuted)
			if sub != "" {
				ui.Text(c, sub).FontSize(fsCaption).
					Font("monospace").TextColor(t.TextMuted).SingleLine()
			}
		})
	})
}

// splitCountsDesc 描述已下载的分流规则规模（数量来自后台探测的 vm）。
func (a *app) splitCountsDesc() string {
	n, m := a.vm.domainCount, a.vm.ipCount
	switch {
	case n > 0 && m > 0:
		return fmt.Sprintf(tr("%d 个域名 / %d 条 IP", "%d domains / %d IPs"), n, m)
	case n > 0 || m > 0:
		return tr("规则不完整", "Incomplete rules")
	default:
		return tr("未下载", "Not downloaded")
	}
}

// dataDirRow 是「数据目录」设置行：路径、打开、更改、恢复默认。
//
// 目录变更必须重启才生效（各组件在启动时持有路径），
// 所以改完后只提示，并提供「立即重启」。
func (a *app) dataDirRow(c *ui.Context) {
	t := c.Theme()

	settingRow(c, tr("数据目录", "Data directory"),
		dataDir+"  ·  "+a.dataDirSrc, func() {
			ui.Row(c).Gap(sp2).Children(func() {
				ui.Button(c, tr("打开", "Open")).OnClick(func() {
					mygo.Shell.OpenPath(dataDir)
				})
				ui.Button(c, tr("更改…", "Change…")).OnClick(a.pickDataDir)
				if a.dataDirSrc == "自定义目录" {
					ui.Button(c, tr("默认", "Default")).OnClick(a.resetDataDir)
				}
			})
		})

	// 回退原因（自定义目录不可用）或网络位置提醒
	if a.dataDirWarn != "" {
		ui.Text(c, a.dataDirWarn).FontSize(fsCaption).TextColor(t.Warning)
	}
	if isNetworkPath(dataDir) {
		ui.Text(c, tr("该目录位于网络位置，可能无法启动内置 gost；"+
			"请改用本地磁盘",
			"Network location: the bundled gost may not run; use a local disk")).
			FontSize(fsCaption).TextColor(t.Warning)
	}
	if len(dataDir) > 180 {
		ui.Text(c, tr("路径过长，Windows 下可能触发 260 字符上限，建议换一个更短的目录",
			"Path is long; Windows may hit the 260-character limit")).
			FontSize(fsCaption).TextColor(t.Warning)
	}

}

// pickDataDir 让用户选一个新目录，进入待确认状态。
func (a *app) pickDataDir() {
	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:             tr("选择数据目录", "Choose data directory"),
		Directory:         true,
		CreateDirectories: true,
		DefaultPath:       filepath.Dir(dataDir),
	})
	if err != nil || len(dirs) == 0 {
		return
	}
	next := dirs[0]
	if err := validateDataDirTarget(dataDir, next); err != nil {
		a.toast(err.Error(), "error")
		return
	}
	a.win.Update(func() { a.pendingDataDir = cleanDirEnv(next) })
}

// applyDataDir 落实目录变更：可选迁移，然后写指针。
//
// 在后台 goroutine 执行：停 gost 最多等 3 秒，迁移还要复制几十 MB，
// 放在主线程会让界面卡住。
func (a *app) applyDataDir(next string, migrate bool) {
	if next == "" {
		return
	}
	// 迁移期间不能再有连接/断开操作在跑：stopGost 在操作槽被占用时
	// 是静默返回的，继续迁移会让 gost 留在旧目录。
	if !a.beginOp() {
		a.toast(tr("正在连接或断开 gost，请稍后再迁移数据目录",
			"Connect/disconnect in progress; try changing the data directory again"), "error")
		return
	}
	defer a.endOp()
	a.win.Update(func() { a.pendingDataDir = "" })

	if st, _, _ := a.pm.State(); st == proc.Running {
		if !a.stopGostLocked() {
			return
		}
	}

	if migrate {
		n, err := migrateDataDir(dataDir, next)
		if err != nil {
			a.toast(tr("迁移失败: ", "Migration failed: ")+err.Error(), "error")
			return
		}
		log.Printf("数据目录迁移：%d 个文件 → %s", n, next)
	}
	if err := writePointer(next); err != nil {
		a.toast(tr("保存失败: ", "Save failed: ")+err.Error(), "error")
		return
	}
	a.win.Update(func() { a.dataDirRestart = true })
	a.toast(tr("重启后生效", "Takes effect after restart"), "success")
}

// resetDataDir 删除指针，回到系统默认目录。
func (a *app) resetDataDir() {
	if err := clearPointer(); err != nil {
		a.toast(tr("恢复失败: ", "Reset failed: ")+err.Error(), "error")
		return
	}
	a.win.Update(func() { a.dataDirRestart = true })
	a.toast(tr("已恢复默认目录，重启后生效", "Default restored. Restart to apply"), "success")
}

// dataDirPendingBlock 渲染目录变更的确认区与重启提示。
// 放在「关于」卡片末尾，不打断上面的信息行。
func (a *app) dataDirPendingBlock(c *ui.Context) {
	t := c.Theme()

	if a.pendingDataDir != "" {
		ui.Divider(c)
		ui.Column(c).FillWidth().Gap(sp2).Children(func() {
			ui.Text(c, tr("更改数据目录", "Change data directory")).FontSize(fsBody).Bold()
			ui.Text(c, a.pendingDataDir).FontSize(fsCaption).
				Font("monospace").TextColor(t.TextMuted).SingleLine()
			ui.Text(c, tr("迁移会把现有配置档、分流规则、gost 与日志复制过去（不删除原目录）；"+
				"不迁移则新目录从空白开始。",
				"Migrate copies profiles, rules, gost and logs (originals kept). "+
					"Without migrating the new directory starts empty.")).
				FontSize(fsCaption).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Gap(sp2).Children(func() {
				ui.Button(c, tr("取消", "Cancel")).Grow(1).OnClick(func() {
					a.pendingDataDir = ""
				})
				ui.Button(c, tr("仅更改", "Change only")).Grow(1).OnClick(func() {
					next := a.pendingDataDir
					go a.applyDataDir(next, false)
				})
				ui.PrimaryButton(c, tr("迁移并更改", "Migrate")).Grow(2).OnClick(func() {
					next := a.pendingDataDir
					go a.applyDataDir(next, true)
				})
			})
		})
	}

	if a.dataDirRestart {
		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).Children(func() {
			ui.Text(c, tr("数据目录已保存，重启后生效", "Saved. Restart to apply")).
				FontSize(fsCaption).TextColor(t.TextMuted).Grow(1)
			ui.PrimaryButton(c, tr("立即重启", "Restart now")).OnClick(func() {
				mygo.App.Relaunch()
			})
		})
	}
}
