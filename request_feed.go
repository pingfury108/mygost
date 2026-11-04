package main

import (
	"log"
	"time"

	"mygost/internal/reqlog"
)

// ─── 请求记录的接线与落盘 ──────────────────────────────────────────
//
// 日志行的解析在 internal/reqlog（纯逻辑），这里只负责把子进程输出
// 接到解析器、把解析结果交给持久化。渲染不参与这条链路。

// feedRequest 把一行原始日志喂给请求解析器（由 proc 的日志回调调用）。
//
// 解析出新的请求时会落盘并让界面重绘；节流由 proc 的 notifySoon 负责。
func (a *app) feedRequest(line string) {
	if a.reqs == nil {
		return
	}
	before := a.reqs.Len()
	lastAt := a.lastRequestAt()
	a.reqs.Feed(line)
	if a.reqs.Len() != before || a.lastRequestAt() != lastAt {
		a.reqs.Flush() // 记录有变化：立即落盘（内部按内容去重）
		if a.win != nil {
			a.win.Invalidate()
		}
	}
}

// lastRequestAt 返回最新一条记录的时间，用来判断“只是更新了已有记录”。
func (a *app) lastRequestAt() (t int64) {
	es := a.reqs.Entries(1)
	if len(es) == 0 {
		return 0
	}
	return es[0].At.UnixNano()
}

// initRequestStore 打开请求记录的持久化后端。
func (a *app) initRequestStore(dir string) {
	st, err := reqlog.OpenStore(dir)
	if err != nil {
		log.Printf("请求记录持久化不可用: %v", err)
		return
	}
	a.reqs.SetStore(st)
}

// flushRequests 落盘请求记录（退出前调用）。
func (a *app) flushRequests() {
	if a.reqs == nil {
		return
	}
	a.reqs.Flush()
	if s := a.reqs.Store(); s != nil {
		_ = s.Close()
	}
}

// watchRequests 定期把记录落盘。
//
// 解析出的记录会先在内存里等后补行（node/err），所以不在 Feed 里逐条写；
// 这里每 2 秒做一次，配合 Store 的内容去重，不会重复写同一版本。
func (a *app) watchRequests() {
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			if a.win == nil || a.win.IsDestroyed() {
				return
			}
			a.reqs.Flush()
		}
	}()
}
