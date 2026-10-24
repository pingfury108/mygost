package main

import (
	"github.com/egoist/mygo"
)

// setupAppMenu 设置应用菜单：macOS 菜单栏；Windows/Linux 为窗口菜单栏。
// 「日志」入口带全局快捷键。
func (a *app) setupAppMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		// macOS 应用菜单（Cmd+Q 等）；其他平台自动省略
		{Role: mygo.RoleAppMenu},
		{Role: mygo.RoleEditMenu},
		{Label: "视图", Submenu: []*mygo.MenuItem{
			{Label: "日志", Accelerator: "CmdOrCtrl+L", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.showLogs()
			}},
			{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.showSettings()
			}},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

// showLogs 切换到日志页（菜单/快捷键共用）。
func (a *app) showLogs() {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.page = "logs" })
	a.win.Show()
	a.win.Focus()
}

// showSettings 切换到设置页。
func (a *app) showSettings() {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.page = "settings" })
	a.win.Show()
	a.win.Focus()
}
