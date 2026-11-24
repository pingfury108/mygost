package main

import (
	"fmt"
	"testing"
	"time"
)

// 请求日志解析不能在 gost 的输出 goroutine 上做磁盘同步：
// 每条记录一次 fsync 会堵住 stdout 管道，把代理吞吐从千级打到几十。
func TestFeedRequestDoesNotSyncPerLine(t *testing.T) {
	a := newTestApp(t)
	defer a.reqs.Store().Close()

	const lines = 4000 // 约 1000 个请求
	start := time.Now()
	for i := 0; i < lines; i++ {
		a.feedRequest(requestLine(i))
	}
	elapsed := time.Since(start)
	t.Logf("%d 行耗时 %v（%.2fms/行）", lines, elapsed,
		float64(elapsed.Microseconds())/lines/1000)

	if elapsed > 2*time.Second {
		t.Fatalf("%d 行耗时 %v，日志回调仍在同步落盘", lines, elapsed)
	}

	// 落盘由后台周期任务负责，主动 Flush 后应能看到记录。
	a.reqs.Flush()
	entries, err := a.reqs.Store().Load(time.Now().Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("Flush 后请求记录未落盘")
	}
}

func requestLine(i int) string {
	sid := fmt.Sprintf("s%d", i/4)
	switch i % 4 {
	case 0:
		return `{"level":"debug","msg":"bypass: 192.168.1.10:8080, whitelist: false","sid":"` + sid + `"}`
	case 1:
		return `{"level":"debug","msg":"proxy: example.com:80, whitelist: false","sid":"` + sid + `"}`
	case 2:
		return `{"level":"debug","msg":"route(retry=0) node-0@1.2.3.4:8081 > example.com:80","sid":"` + sid + `"}`
	default:
		return `{"level":"debug","msg":"route(retry=0) 192.168.1.10:8080","sid":"` + sid + `"}`
	}
}
