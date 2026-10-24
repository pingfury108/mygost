package main

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mygost/internal/settings"
)

// viewSettings 设置页：改动即时生效，改动后有短时"已保存"反馈。
func (a *app) viewSettings(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(sp4).Gap(sp3).Children(func() {
			// ── gost 程序 ──
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					sectionTitle(c, "GOST 程序")

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
							ui.Button(c, "下载 / 更新").OnClick(func() { go a.doDownload() })
						}
					})
				})
			})

			// ── 行为 ──
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					sectionTitle(c, "行为")
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
					divider(c)
					settingRow(c, "退出时保持运行", "关闭 mygost 后 gost 继续工作", func() {
						if ui.Switch(c, &a.sKeep).Changed() {
							a.saveSettings()
						}
					})
				})
			})

			// ── 分流 ──
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp3).Children(func() {
					sectionTitle(c, tr("分流", "Routing"))
					settingRow(c, tr("国内直连", "Direct for CN"),
						tr("命中国内 IP 的目标直接连接，其余走隧道",
							"CN IP targets connect directly, others via tunnel"), func() {
							if ui.Switch(c, &a.sSplit).Changed() {
								a.onSplitToggled()
							}
						})
					if a.sSplit {
						ui.Divider(c)
						settingRow(c, tr("IP 列表", "IP List"), a.ipListDesc(), func() {
							if a.splitBusy {
								ui.Spinner(c)
							} else {
								ui.Button(c, tr("立即更新", "Update")).OnClick(func() { go a.updateIPList() })
							}
						})
					}
				})
			})

			// ── 关于 ──
			card(c).Children(func() {
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					sectionTitle(c, "关于")
					kvRow(c, "应用", "mygost 0.1.0 · gost 桌面客户端")
					kvRow(c, "数据目录", dataDir)
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
	})
	a.savedUntil = time.Now().Add(2 * time.Second)
}

// saveSettingsDebounced 文本输入防抖：停止输入 600ms 后才落盘。
func (a *app) saveSettingsDebounced() {
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(600*time.Millisecond, a.saveSettings)
}

// saveAndCheck 落盘并重新检测 gost 二进制（路径/镜像变化时用）。
func (a *app) saveAndCheck() {
	a.saveSettings()
	a.checkBinAsync()
}

// ipListDesc 是 IP 列表行的描述文字。
func (a *app) ipListDesc() string {
	if a.splitBusy {
		return tr("更新中…", "Updating…")
	}
	n := a.geoM.CountLines()
	if n < 0 {
		return tr("尚未下载", "Not downloaded")
	}
	at := a.geoM.UpdateAt().Format("2006-01-02 15:04")
	return fmt.Sprintf("%d 条 · %s", n, at)
}

// onSplitToggled 在开关变化时落盘；开启即预下载规则，保证下次连接可用。
func (a *app) onSplitToggled() {
	a.saveSettings()
	if a.sSplit && !a.geoM.ListReady() && !a.splitBusy {
		go a.updateIPList()
	}
}

// updateIPList 在后台更新国内 IP 列表。
func (a *app) updateIPList() {
	a.win.Update(func() { a.splitBusy = true })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	n, err := a.geoM.UpdateIPList(ctx, a.sSplitURL)
	a.win.Update(func() {
		a.splitBusy = false
		if err != nil {
			a.pendingToasts = append(a.pendingToasts,
				ui.Toast{Title: tr("更新失败: ", "Update failed: ") + err.Error(), Type: "error"})
			return
		}
		a.pendingToasts = append(a.pendingToasts,
			ui.Toast{Title: fmt.Sprintf(tr("已更新 %d 条", "Updated %d entries"), n), Type: "success"})
	})
}
