package main

import (
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/reqlog"
	"mygost/internal/settings"
)

// 渲染路径必须廉价：滚动时每秒几十帧，不能有 exec/dial/读大文件。
//
// 曾经设置页每帧都 fork 一次 pgrep、拨一次 TCP、读 11 万行列表，
// 单帧 50ms 以上，滚起来像幻灯片。
func TestSettingsFrameIsCheap(t *testing.T) {
	if raceEnabled {
		t.Skip("帧耗时阈值在 -race 下会被放大，性能测试只在普通模式运行")
	}
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

// pgrep 很贵，残留进程扫描必须走缓存，不能每次探测都重新扫进程表。
func TestLeftoverScanIsCached(t *testing.T) {
	a := newTestApp(t)
	a.viewLeftovers()
	if !a.leftoverOK {
		t.Fatal("首次探测应写入缓存")
	}
	a.invalidateLeftovers()
	if a.leftoverOK {
		t.Fatal("invalidate 后应要求重新扫描")
	}
}

// 请求落盘每 500ms 一次。环形缓冲满后，没有新记录时
// Flush 必须是 O(1) 附近的空操作，不能每次把全部记录重新格式化/查重。
func TestFlushCostBounded(t *testing.T) {
	if raceEnabled {
		t.Skip("帧耗时阈值在 -race 下会被放大，性能测试只在普通模式运行")
	}
	a := newTestApp(t)
	a.reqs = reqlog.New(5000)
	a.initRequestStore(t.TempDir())
	defer a.reqs.Store().Close()

	for i := 0; i < 5000; i++ {
		a.feedRequest(requestLine(i))
	}
	a.reqs.Flush() // 首次落盘

	const n = 20
	start := time.Now()
	for i := 0; i < n; i++ {
		a.reqs.Flush()
	}
	per := time.Since(start) / n
	t.Logf("5000 条缓冲空转 Flush：%v/次", per)
	if per > 5*time.Millisecond {
		t.Errorf("空转 Flush %v 过慢，高频下会持续占用 CPU", per)
	}
}
