package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/reqlog"
)

// 请求视图的渲染路径不能碰磁盘：历史 JSONL 可达数 MB，
// 每帧重读会让页面掉到十几帧。
func TestRequestViewsAreCheap(t *testing.T) {
	if raceEnabled {
		t.Skip("帧耗时阈值在 -race 下会被放大，性能测试只在普通模式运行")
	}
	a := newTestApp(t)

	// 1) 请求历史：4MB JSONL。只应在选中日期时加载一次。
	dir := t.TempDir()
	writeDayFile(t, dir, time.Now().AddDate(0, 0, -1), 30000)
	st, err := reqlog.OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.reqs.SetStore(st)
	a.reqDay = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	a.loadRequestDay(a.reqDay)
	a.refreshViewModel()
	a.page, a.logView = "logs", 1
	hist := frameCost(a.view, 10)
	t.Logf("请求历史页：%v/帧", hist)
	if hist > 30*time.Millisecond {
		t.Errorf("历史页 %v/帧，渲染路径仍在读文件", hist)
	}

	// 2) 今日记录：store 尚未写入过，今日计数不能每帧扫描文件。
	dir2 := t.TempDir()
	writeDayFile(t, dir2, time.Now(), 30000)
	st2, err := reqlog.OpenStore(dir2)
	if err != nil {
		t.Fatal(err)
	}
	a.reqs.SetStore(st2)
	a.reqDay = ""
	a.refreshViewModel()
	live := frameCost(a.view, 10)
	t.Logf("请求实时页：%v/帧", live)
	if live > 15*time.Millisecond {
		t.Errorf("实时页 %v/帧，计数/日期列表仍在渲染路径", live)
	}
}

func frameCost(view func(c *ui.Context), n int) time.Duration {
	tt := ui.NewTester(view, 430, 660)
	tt.Frame() // 预热
	tt.Image()
	start := time.Now()
	for i := 0; i < n; i++ {
		tt.Frame()
		tt.Image()
	}
	return time.Since(start) / time.Duration(n)
}

func writeDayFile(t *testing.T, dir string, at time.Time, n int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `{"key":"s%d","at":%q,"target":"www.example-%d.com:443","decision":2,"node":"node-0@1.2.3.4:8081"}`+"\n",
			i, at.Add(time.Duration(i)*time.Millisecond).Format(time.RFC3339Nano), i%100)
	}
	path := filepath.Join(dir, at.Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	t.Logf("%s: %.1f MB", filepath.Base(path), float64(st.Size())/1e6)
}
