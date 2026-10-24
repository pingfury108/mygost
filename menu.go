package main

import (
	"github.com/egoist/mygo"
)

// setupAppMenu 设置应用菜单：macOS 菜单栏；Windows/Linux 为窗口菜单栏。
// 文案跟随系统语言，快捷键固定。
func (a *app) setupAppMenu() {
	detectLang()
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		// macOS 应用菜单（Quit 等）；其他平台自动省略
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: tr("视图", "View"), Submenu: []*mygo.MenuItem{
			{Label: tr("日志", "Logs"), Accelerator: "CmdOrCtrl+L", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.showLogs()
			}},
			{Label: tr("设置…", "Settings…"), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.showSettings()
			}},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

// showMain 把主窗口带到前台。
func (a *app) showMain() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Focus()
}

// showLogs 切换到日志页（菜单/快捷键共用）。
func (a *app) showLogs() {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.page = "logs" })
	a.showMain()
}

// showSettings 切换到设置页。
func (a *app) showSettings() {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.page = "settings" })
	a.showMain()
}
