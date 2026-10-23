package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mygost/internal/gostbin"
	"mygost/internal/settings"
)

// viewSettings 设置页：按"程序 / 行为 / 关于"分组卡片。
func (a *app) viewSettings(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
			// ── gost 程序 ──
			card(c).Children(func() {
				ui.Column(c).Fill().Gap(12).Children(func() {
					sectionTitle(c, "gost 程序")
					settingRow(c, "本地路径", "留空自动查找", func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.TextInput(c, &a.sGostPath).Grow(1).Placeholder("/usr/local/bin/gost")
							ui.Button(c, "…").OnClick(a.browseGost)
						})
					})
					ui.Divider(c)
					settingRow(c, "下载镜像", "支持 {version} {os} {arch} {ext}", func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.TextInput(c, &a.sMirror).Grow(1).Placeholder(gostbin.DefaultMirror)
							ui.Button(c, "默认").OnClick(func() { a.sMirror = "" })
						})
					})
					if a.binVer != "" {
						kvRow(c, "版本", a.binVer)
					}
				})
			})

			// ── 行为 ──
			card(c).Children(func() {
				ui.Column(c).Fill().Gap(12).Children(func() {
					sectionTitle(c, "行为")
					settingRow(c, "开机自启", "登录后自动运行 mygost", func() {
						if ui.Switch(c, &a.sLogin).Changed() {
							if err := mygo.App.SetOpenAtLogin(a.sLogin); err != nil {
								a.toast("设置失败: "+err.Error(), "error")
							}
						}
					})
					ui.Divider(c)
					settingRow(c, "自动运行", "启动后连接当前配置", func() {
						ui.Switch(c, &a.sAuto)
					})
					ui.Divider(c)
					settingRow(c, "退出时保持运行", "关闭 mygost 后 gost 继续工作", func() {
						ui.Switch(c, &a.sKeep)
					})
				})
			})

			// ── 保存 ──
			ui.PrimaryButton(c, "保存设置").OnClick(a.saveSettings)

			// ── 关于 ──
			ui.Text(c, "mygost 0.1.0 · gost 桌面客户端").FontSize(12).TextColor(c.Theme().TextMuted)
		})
	})
}

// browseGost 打开文件选择框。
func (a *app) browseGost() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择 gost 程序"})
	if err != nil || len(paths) == 0 {
		return
	}
	a.win.Update(func() { a.sGostPath = paths[0] })
}

// saveSettings 落盘设置页表单。
func (a *app) saveSettings() {
	a.update(func(s *settings.Settings) {
		s.GostPath = a.sGostPath
		s.Mirror = a.sMirror
		s.KeepOnExit = a.sKeep
		s.AutoStart = a.sAuto
	})
	a.toast("设置已保存", "success")
	a.checkBinAsync()
}
