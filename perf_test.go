package main

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/settings"
)

// 渲染路径必须廉价：滚动时每秒几十帧，不能有 exec/dial/读大文件。
//
// 曾经设置页每帧都 fork 一次 pgrep、拨一次 TCP、读 11 万行列表，
// 单帧 50ms 以上，滚起来像幻灯片。
func TestSettingsFrameIsCheap(t *testing.T) {
	a := newTestApp(t)

	// 造 11 万行的域名表，模拟真实规模
	f, err := os.Create(a.geoM.DomainListPath())
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, 1<<20)
	for i := 0; i < 110000; i++ {
		buf = append(buf, "some-domain-suffix-for-testing.com\n"...)
	}
	f.Write(buf)
	f.Close()
	os.WriteFile(a.geoM.IPListPath(), []byte("1.0.0.0/8\n"), 0o644)

	a.st.Update(func(s *settings.Settings) { s.Split = true })
	a.sSplit = true
	a.page = "settings"
	a.refreshViewModel() // 探测一次性做完，之后渲染只读 vm

	tt := ui.NewTester(a.view, 430, 660)
	const frames = 60
	start := time.Now()
	for i := 0; i < frames; i++ {
		tt.Scroll(215, 400, 0, -20)
		tt.Image()
	}
	per := time.Since(start) / frames
	t.Logf("平均每帧 %v", per)
	if per > 8*time.Millisecond {
		t.Errorf("单帧 %v 过高，滚动会卡（目标 < 8ms）", per)
	}
}

// 探测本身可以慢（fork pgrep、拨端口），因为它不在渲染路径上，
// 但也要有个上界，否则 2 秒周期会被拖长。
func TestProbeCostBounded(t *testing.T) {
	a := newTestApp(t)
	const n = 5
	start := time.Now()
	for i := 0; i < n; i++ {
		a.probeViewModel()
	}
	per := time.Since(start) / n
	t.Logf("单次探测 %v", per)
	if per > 500*time.Millisecond {
		t.Errorf("探测 %v 过慢，状态会明显滞后", per)
	}
}
