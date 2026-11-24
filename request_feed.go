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
// 这里只更新内存模型并请求重绘；落盘交给 watchRequests 周期性合并，
// 否则每个请求一次 fsync 会堵住 gost 的 stdout 管道（实测 21ms/请求）。
func (a *app) feedRequest(line string) {
	if a.reqs == nil {
		return
	}
	if a.reqs.Feed(line) {
		a.invalidateRequestView()
	}
}

// reqInvalidateInterval 是请求视图重绘的最小间隔。
// debug 日志约 11 行/请求，逐条 Invalidate 会让界面一直在重建。
const reqInvalidateInterval = 200 * time.Millisecond

func (a *app) invalidateRequestView() {
	w := a.win
	if w == nil {
		return
	}
	now := time.Now().UnixNano()
	last := a.reqInvalidateAt.Load()
	if now-last >= int64(reqInvalidateInterval) {
		if a.reqInvalidateAt.CompareAndSwap(last, now) {
			w.Invalidate()
			return
		}
	}
	// 节流窗口内仍有新记录：补一次尾随重绘。否则流量恰好停下时，
	// 最后一条请求要等下一次 2 秒状态轮询才出现。
	if a.reqInvalidateQueued.CompareAndSwap(false, true) {
		time.AfterFunc(reqInvalidateInterval, func() {
			a.reqInvalidateQueued.Store(false)
			a.reqInvalidateAt.Store(time.Now().UnixNano())
			if w := a.win; w != nil && !w.IsDestroyed() {
				w.Invalidate()
			}
		})
	}
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

const (
	requestFlushInterval  = 500 * time.Millisecond
	requestHistoryEvery   = 4 // 每 4 次落盘（2s）检查一次历史视图
	requestViewMaxEntries = 500
)

// watchRequests 定期把记录落盘。
//
// 解析出的记录会先在内存里等后补行（node/err），所以不在 Feed 里逐条写；
// 这里每 500ms 做一次（配合脏记录标记，只写变化过的），Store 再做内容去重。
// 写文件不带 fsync（见 reqlog.Store.flushLocked），不会反压日志读取。
func (a *app) watchRequests() {
	go func() {
		t := time.NewTicker(requestFlushInterval)
		defer t.Stop()
		ticks := 0
		for range t.C {
			if a.win == nil || a.win.IsDestroyed() {
				return
			}
			a.reqs.Flush()
			ticks++
			if ticks%requestHistoryEvery != 0 {
				continue
			}
			// 用户正在看"今天"的落盘记录时，文件会持续变化；
			// 仅在线数变化时后台重载，过去日期不产生任何开销。
			a.applyUI(func() {
				// 只有真正停在请求视图上才值得重载（历史文件可能几 MB）。
				// 用户切走后 reqDay 仍保留，但不应继续在后台解析。
				if a.page != "logs" || a.logView != 1 {
					return
				}
				if a.reqDay == "" || a.reqDay != todayKey() {
					return
				}
				if st := a.reqs.Store(); st != nil && st.DayCount(a.reqDay) != a.reqDayCount {
					a.reqDayLoaded = ""
				}
			})
		}
	}()
}
