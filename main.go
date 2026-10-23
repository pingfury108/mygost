package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
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
	formKind   int  // Segmented 索引
	formUpOpen bool // 上游转发链折叠区
	customBuf  *editor.State

	// 日志页
	logFilter string

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
}

var dataDir string

func main() {
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := newApp()

	mygo.App.WhenReady(func() {
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
		a.setupTray()
		a.refreshTray()
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
		page: "main",
		st:   st,
		ps:   ps,
		loc:  &gostbin.Locator{Dir: filepath.Join(dataDir, "bin")},
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
