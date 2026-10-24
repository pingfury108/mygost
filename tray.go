package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/egoist/mygo"
	"mygost/internal/proc"
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
}

// refreshTray 按当前进程状态更新托盘菜单与图标。
func (a *app) refreshTray() {
	if a.tray == nil {
		return
	}
	state, _, _ := a.pm.State()
	running := state == proc.Running
	// 运行中实心拱洞，停止时空心
	a.tray.SetIcon(trayPNG(running), true)

	action := "连接"
	if running {
		action = "断开"
	}
	items := []*mygo.MenuItem{
		{Label: "打开 mygost", Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.win != nil {
				a.win.Show()
				a.win.Focus()
			}
		}},
		mygo.Separator(),
		{Label: action, Click: func(*mygo.MenuItem, *mygo.Window) {
			if running {
				go a.pm.Stop()
			} else {
				go a.doStart()
			}
		}},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}
	a.tray.SetMenu(mygo.NewMenu(items))
}

// trayPNG 绘制 32×32 单色托盘图标：隧道拱洞，运行中实心并带光点。
func trayPNG(running bool) []byte {
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
	// 拱洞：上为半圆，下为直边
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
	// 中心光点
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
					hit := inArch(x, y, halfW) && !inArch(x, y, halfW-wall) // 拱壁
					if running {
						hit = inArch(x, y, halfW) || inDot(x, y) // 实心 + 光点
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
