//go:build ignore

// icongen 生成应用图标 resources/icon.png（1024×1024）。
//
// 运行：go run tools/icongen/main.go
//
// 图形：蓝色渐变圆角方块 + 白色隧道拱洞 + 洞口的光点。
// "拱洞"是 gost（GO Simple Tunnel）最直接的隐喻：数据从这里穿过。
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// 超采样倍数：4×4 子采样，边缘足够平滑。
const (
	size = 1024
	ss   = 4
)

func main() {
	n := size * ss
	// 覆盖率掩码
	bg := make([]bool, n*n)   // 圆角方块底
	arch := make([]bool, n*n) // 拱洞外形
	hole := make([]bool, n*n) // 拱洞内孔（挖空）
	dot := make([]bool, n*n)  // 洞口光点

	// 几何（按 1024 画布定义，再乘以 ss）
	const (
		margin = 56.0  // 方块外边距
		radius = 228.0 // 方块圆角
	)
	inRoundedRect := func(x, y, x0, y0, x1, y1, r float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx := clamp(x, x0+r, x1-r)
		cy := clamp(y, y0+r, y1-r)
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
	// 拱洞：上半为半圆，下半为直边（隧道断面）
	archShape := func(x, y, cx, bottom, halfW, top float64) bool {
		if y < top || y > bottom {
			return false
		}
		if y >= top+halfW { // 直边段
			return math.Abs(x-cx) <= halfW
		}
		dy := y - (top + halfW)
		return (x-cx)*(x-cx)+dy*dy <= halfW*halfW
	}
	// 内孔在底端收圆角，视觉上像石拱而非字母 n
	archRounded := func(x, y, cx, bottom, halfW, top, r float64) bool {
		if !archShape(x, y, cx, bottom, halfW, top) {
			return false
		}
		if y > bottom-r && math.Abs(x-cx) > halfW-r {
			cxx := cen(x, cx, halfW, r, y, bottom)
			dx, dy := x-cxx, y-(bottom-r)
			return dx*dx+dy*dy <= r*r
		}
		return true
	}

	const (
		cx     = 512.0
		bottom = 800.0
		top    = 244.0
		halfW  = 214.0
		wall   = 86.0 // 拱壁厚度
	)

	for py := 0; py < n; py++ {
		for px := 0; px < n; px++ {
			x := (float64(px) + 0.5) / ss
			y := (float64(py) + 0.5) / ss
			i := py*n + px
			bg[i] = inRoundedRect(x, y, margin, margin, size-margin, size-margin, radius)
			arch[i] = archRounded(x, y, cx, bottom, halfW, top, 22)
			hole[i] = archRounded(x, y, cx, bottom+10, halfW-wall, top+wall, 12)
			// 洞口光点：居中偏上，像刚穿出来的数据
			dx, dy := x-cx, y-512
			dot[i] = dx*dx+dy*dy <= 56*56
		}
	}

	out := image.NewRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					i := (py*ss+sy)*n + px*ss + sx
					x := (float64(px*ss+sx) + 0.5) / ss
					y := (float64(py*ss+sy) + 0.5) / ss
					c, ca := sample(i, x, y, bg, arch, hole, dot)
					r += float64(c.R) * ca
					g += float64(c.G) * ca
					b += float64(c.B) * ca
					a += ca
				}
			}
			w := float64(ss * ss)
			r, g, b, a = r/w, g/w, b/w, a/w
			if a > 0 { // 未预乘
				r, g, b = r/a, g/a, b/a
			}
			out.SetRGBA(px, py, color.RGBA{
				R: to8(r), G: to8(g), B: to8(b), A: to8(a * 255),
			})
		}
	}

	f, err := os.Create("resources/icon.png")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		panic(err)
	}
}

// sample 返回该子采样点的颜色与覆盖度。
func sample(i int, x, y float64, bg, arch, hole, dot []bool) (color.RGBA, float64) {
	if arch[i] && !hole[i] {
		// 拱壁：白色，顶部提亮形成体积感
		t := clamp(y/1024.0, 0, 1)
		v := 255.0 - 18*t
		return color.RGBA{to8(v), to8(v), to8(v), 255}, 1
	}
	if dot[i] {
		return color.RGBA{125, 211, 252, 255}, 1 // 浅青：数据流（叠加在洞里）
	}
	if bg[i] {
		// 对角渐变：左上亮蓝 → 右下深蓝
		t := clamp((x+y-2*256)/1024, 0, 1)
		return mix(color.RGBA{59, 130, 246, 255}, color.RGBA{29, 78, 216, 255}, t), 1
	}
	return color.RGBA{}, 0
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		to8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		to8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		to8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		255,
	}
}

// cen 求底端圆角段的圆心 x。
func cen(x, cx, halfW, r, y, bottom float64) float64 {
	if x < cx {
		return cx - halfW + r
	}
	return cx + halfW - r
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func to8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}
