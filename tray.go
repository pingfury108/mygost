package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"github.com/egoist/mygo"
	"mygost/internal/proc"
)

// setupTray 创建托盘图标（运行中实心圆点，停止时空心圆点）。
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
	a.tray.SetIcon(trayPNG(running), true)

	startLabel := "启动"
	if running {
		startLabel = "停止"
	}
	items := []*mygo.MenuItem{
		{Label: "打开 mygost", Click: func(*mygo.MenuItem, *mygo.Window) {
			if a.win != nil {
				a.win.Show()
				a.win.Focus()
			}
		}},
		{Label: startLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
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

// trayPNG 绘制 32x32 托盘图标：黑色圆点（模板图标，系统自动着色）。
func trayPNG(running bool) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	black := color.RGBA{0, 0, 0, 255}
	cx, cy := float64(size)/2, float64(size)/2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := (float64(x)-cx+0.5)*(float64(x)-cx+0.5) + (float64(y)-cy+0.5)*(float64(y)-cy+0.5)
			switch {
			case running && d <= 100: // r=10 实心
				img.Set(x, y, black)
			case !running && d <= 100 && d >= 56: // 空心环
				img.Set(x, y, black)
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
