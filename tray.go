package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"mygost/internal/proc"
	"mygost/internal/settings"
)

// setupTray 创建托盘图标（与 app 图标同一标记：隧道拱洞）。
func (a *app) setupTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "main",
		Icon:           trayPNG(false),
		IconIsTemplate: true,
		ToolTip:        "mygost",
	})
	if err != nil {
		return
	}
	a.tray = tray
	a.refreshTray()
	a.tickTray()
}

// tickTray 运行中每秒刷新一次托盘（菜单栏里的运行时长）。
func (a *app) tickTray() {
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			if a.win == nil || a.win.IsDestroyed() {
				return
			}
			if st, _, _ := a.pm.State(); st == proc.Running {
				a.applyUI(a.refreshTray)
			}
		}
	}()
}

// refreshTray 同步托盘图标、菜单栏文字、提示与菜单。
func (a *app) refreshTray() {
	if a.tray == nil {
		return
	}
	state, started, _ := a.pm.State()
	running := state == proc.Running

	// 图标：运行中实心拱洞，停止时空心
	if !a.trayIconSet || a.trayIconRunning != running {
		a.tray.SetIcon(trayPNG(running), true)
		a.trayIconRunning, a.trayIconSet = running, true
	}

	name, port := a.trayTarget()

	// 只保留图标：菜单栏文字会让菜单栏显得杂乱，状态放进提示与菜单里
	// 悬停提示：状态 + 配置 + 端口 + 时长
	a.tray.SetToolTip(trayTooltip(running, state, name, port, started))

	a.tray.SetMenu(mygo.NewMenu(a.trayMenuItems(running, state, name, port, started)))
}

// trayTarget 返回当前选中的配置名与端口。
func (a *app) trayTarget() (name, port string) {
	id := a.runningProfileID()
	if id == "" {
		id = a.st.Get().Current
	}
	if p := a.ps.Get(id); p != nil {
		return p.Name, p.Listen
	}
	return "", ""
}

// trayTooltip 是鼠标悬停时的完整状态。
func trayTooltip(running bool, state proc.State, name, port string, started time.Time) string {
	switch {
	case running:
		parts := tr("运行中", "Connected")
		if name != "" {
			parts += " · " + name
		}
		if port != "" {
			parts += " · " + port
		}
		if !started.IsZero() {
			parts += " · " + fmtDuration(time.Since(started))
		}
		return "mygost " + parts
	case state == proc.Crashed:
		return "mygost " + tr("异常退出", "crashed")
	default:
		return "mygost " + tr("未连接", "disconnected")
	}
}

// trayMenuItems 构建托盘菜单。
func (a *app) trayMenuItems(running bool, state proc.State, name, port string, started time.Time) []*mygo.MenuItem {
	// 状态行：只读
	status := tr("未连接", "Disconnected")
	switch {
	case running:
		status = tr("运行中 ", "Connected ") + fmtDuration(time.Since(started))
		if name != "" {
			status += " · " + name
		}
		if port != "" {
			status += " " + port
		}
	case state == proc.Crashed:
		status = tr("异常退出", "Crashed")
	}

	action := tr("连接", "Connect")
	if running {
		action = tr("断开", "Disconnect")
	}

	return []*mygo.MenuItem{
		{Label: status, Disabled: true},
		mygo.Separator(),
		{Label: action, Click: func(*mygo.MenuItem, *mygo.Window) {
			if running {
				go a.stopGost()
			} else {
				go a.doStart()
			}
		}},
		mygo.Separator(),
		{Label: tr("配置", "Profiles"), Submenu: a.trayProfileItems()},
		mygo.Separator(),
		{Label: tr("日志…", "Logs…"), Accelerator: "CmdOrCtrl+L", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showLogs()
		}},
		{Label: tr("设置…", "Settings…"), Accelerator: "CmdOrCtrl+,", Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showSettings()
		}},
		mygo.Separator(),
		{Label: tr("打开 mygost", "Open mygost"), Click: func(*mygo.MenuItem, *mygo.Window) {
			a.showMain()
		}},
		{Role: mygo.RoleQuit},
	}
}

// trayProfileItems 是"配置"子菜单：单选，选中项即运行中/待连接的配置。
func (a *app) trayProfileItems() []*mygo.MenuItem {
	items := a.ps.Items()
	if len(items) == 0 {
		return []*mygo.MenuItem{{Label: tr("还没有配置", "No profiles yet"), Disabled: true}}
	}
	current := a.st.Get().Current
	if rid := a.runningProfileID(); rid != "" {
		current = rid
	}
	out := make([]*mygo.MenuItem, 0, len(items)+1)
	for _, p := range items {
		p := p
		out = append(out, &mygo.MenuItem{
			Label:   p.Name,
			Type:    mygo.MenuItemRadio,
			Checked: p.ID == current,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				go a.trayConnect(p.ID)
			},
		})
	}
	out = append(out, mygo.Separator())
	out = append(out, &mygo.MenuItem{Label: tr("在窗口中编辑…", "Edit in Window…"), Click: func(*mygo.MenuItem, *mygo.Window) {
		a.showMain()
	}})
	return out
}

// trayConnect 从托盘切换/连接某个配置：
// 未运行则直接连接；运行其它配置则先停后启，失败回滚原配置。
func (a *app) trayConnect(id string) {
	// 与窗口共用同一把启停锁和完整启动流程，避免托盘绕过端口预检、
	// 插件就绪、配置重写与 PID 记录。
	if !a.beginOp() {
		return
	}
	defer a.endOp()

	if a.ps.Get(id) == nil {
		return
	}

	runningID := a.runningProfileID()
	if runningID != "" {
		if runningID == id {
			return // 已在运行的就是它
		}
		if !a.stopGostLocked() {
			return
		}
	}

	oldID := a.st.Get().Current
	if runningID != "" {
		oldID = runningID
	}
	a.update(func(s *settings.Settings) { s.Current = id })
	if a.startCurrent() {
		return
	}

	// 回滚到原先真正在运行的配置；未运行过的配置不自动拉起。
	a.update(func(s *settings.Settings) { s.Current = oldID })
	if runningID != "" && oldID != "" {
		a.startCurrentNotify(false)
	}
}

var trayPNGCache struct {
	once          sync.Once
	idle, running []byte
}

// trayPNG 返回托盘图标；两种状态只生成一次，避免每秒刷新都做
// 超采样与 PNG 编码。
func trayPNG(running bool) []byte {
	trayPNGCache.once.Do(func() {
		trayPNGCache.idle = buildTrayPNG(false)
		trayPNGCache.running = buildTrayPNG(true)
	})
	if running {
		return trayPNGCache.running
	}
	return trayPNGCache.idle
}

// buildTrayPNG 绘制 32×32 单色托盘图标：隧道拱洞，运行中实心并带光点。
func buildTrayPNG(running bool) []byte {
	const (
		out = 32
		ss  = 4
		n   = out * ss
	)
	// 几何（按 32 点坐标系定义）
	const (
		cx     = 16.0
		top    = 4.0
		bottom = 27.0
		halfW  = 9.5
		wall   = 3.6
	)
	inArch := func(x, y, w float64) bool {
		if y < top || y > bottom {
			return false
		}
		if y >= top+w {
			return math.Abs(x-cx) <= w
		}
		dy := y - (top + w)
		return (x-cx)*(x-cx)+dy*dy <= w*w
	}
	inDot := func(x, y float64) bool {
		dx, dy := x-cx, y-19.5
		return dx*dx+dy*dy <= 2.6*2.6
	}

	img := image.NewRGBA(image.Rect(0, 0, out, out))
	black := color.RGBA{0, 0, 0, 255}
	for py := 0; py < out; py++ {
		for px := 0; px < out; px++ {
			covered := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px*ss+sx) + 0.5) / ss
					y := (float64(py*ss+sy) + 0.5) / ss
					hit := inArch(x, y, halfW) && !inArch(x, y, halfW-wall)
					if running {
						hit = inArch(x, y, halfW) || inDot(x, y)
					}
					if hit {
						covered++
					}
				}
			}
			if covered == 0 {
				continue
			}
			c := black
			c.A = uint8(255 * covered / (ss * ss))
			img.SetRGBA(px, py, c)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
