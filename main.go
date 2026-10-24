package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	// MyGo 自身文案（菜单 Edit/View/Window、Undo/Copy 等）的中文翻译，
	// 未 import 时框架字符串永远是英文，导致菜单中英混排。
	_ "github.com/egoist/mygo/locales/zh-Hans"
	_ "github.com/egoist/mygo/locales/zh-Hant"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"mygost/internal/gostbin"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

// app 是应用状态；视图是它的函数。字段只在主线程（或经 Window.Update）读写。
type app struct {
	win  *mygo.Window
	st   *settings.Store
	ps   *profiles.Store
	loc  *gostbin.Locator
	pm   *proc.Manager
	tray *mygo.Tray

	// 页面导航（窄窗口：主界面 + 推入式子页）
	page string // main / logs / settings / editor

	// 主界面
	binPath   string
	binSource string
	binVer    string
	binFound  bool
	dlBusy    bool
	dlDone    int64
	dlTotal   int64

	// 配置档编辑
	editing    bool
	form       profiles.Profile
	formKind   int    // Segmented 索引
	formUpOpen bool   // 上游转发链折叠区
	formErr    string // 表单内联错误
	cmdPreview string // 等效命令（只读，每帧重算）
	customBuf  *editor.State

	// 日志页
	logFilter string
	logFollow bool // 自动跟随最新日志（可关闭）
	logSeen   int  // 上一帧的行数，用于只在新增时滚动

	// 主题安装状态（跟随系统深浅色）
	themeApplied bool
	themeDark    bool

	// 设置页保存反馈（短时提示）
	savedUntil time.Time

	// 待展示的 toast（视图构建时 flush 到窗口）
	pendingToasts []ui.Toast

	// 删除确认
	delOpen   bool
	delTitle  string
	delTarget string

	// 设置页表单
	sGostPath string
	sMirror   string
	sKeep     bool
	sAuto     bool
	sLogin    bool

	saveTimer *time.Timer // 文本输入防抖，停顿后才落盘
}

var dataDir string

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	// 应用以中英两种语言书写：框架内建字符串（菜单角色名等）跟随系统语言
	mygo.App.SetLanguages("en", "zh-Hans")
	a := newApp()
	// 已经有一个实例在跑时（比如窗口被收起后再次启动图标），
	// 把那个实例的窗口叫出来，而不是静默退出让人以为没反应。
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })

	mygo.App.WhenReady(func() {
		// 菜单/托盘文案跟随系统语言；打一行日志便于排查
		loc, zh := currentLocale()
		log.Printf("locale=%q zh=%v", loc, zh)
		mygo.App.OnLocaleChanged(func(locale string) {
			detectLang()
			a.setupAppMenu()
			a.refreshTray()
		})
		// 设置页表单初值
		s := a.st.Get()
		a.sGostPath = s.GostPath
		a.sMirror = s.Mirror
		a.sKeep = s.KeepOnExit
		a.sAuto = s.AutoStart
		a.sLogin = mygo.App.OpenAtLogin()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "mygost",
			Width:     430,
			Height:    660,
			MinWidth:  380,
			MinHeight: 520,
			Content:   ui.View(a.view),
		})
		// 关窗口只是收起：应用继续待在菜单栏，gost 也继续运行。
		// 真正退出走菜单栏「退出 mygost」（⌘Q）。
		a.win.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			a.win.Hide()
		})
		// 点 Dock 图标时把窗口叫回来（macOS 无可见窗口时）
		mygo.App.OnActivate(func(hasVisibleWindows bool) {
			if !hasVisibleWindows {
				a.showMain()
			}
		})

		a.setupTray()
		a.refreshTray()
		a.setupAppMenu()
		a.checkBinAsync()
		if s.AutoStart && s.Current != "" {
			go a.doStart()
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		if !a.st.Get().KeepOnExit {
			a.pm.StopAndWait(3 * time.Second)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// newApp 初始化存储与进程管理器。
func newApp() *app {
	base, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	dataDir = filepath.Join(base, "mygost")

	st, err := settings.Load(dataDir)
	if err != nil {
		log.Printf("读取设置失败: %v", err)
	}
	ps, err := profiles.LoadStore(filepath.Join(dataDir, "profiles"))
	if err != nil {
		log.Printf("读取配置档失败: %v", err)
	}
	a := &app{
		page:      "main",
		logFollow: true,
		st:        st,
		ps:        ps,
		loc:       &gostbin.Locator{Dir: filepath.Join(dataDir, "bin")},
	}
	a.pm = proc.NewManager(a.onProcEvent)
	return a
}

// onProcEvent 在后台 goroutine 被调用：切回主线程刷新界面与托盘。
func (a *app) onProcEvent() {
	if a.win == nil {
		return
	}
	a.win.Update(func() { a.refreshTray() })
}

// update 更新设置并落盘。
func (a *app) update(fn func(*settings.Settings)) {
	if err := a.st.Update(fn); err != nil {
		log.Printf("保存设置失败: %v", err)
	}
}

// toast 在窗口上弹出提示（视图构建时展示）。
func (a *app) toast(title, typ string) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		a.pendingToasts = append(a.pendingToasts, ui.Toast{Title: title, Type: typ})
	})
}
