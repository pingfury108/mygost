package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"mygost/internal/proc"
)

// ─── 分流插件监管器 ────────────────────────────────────────────────
//
// 插件（splitd）的生命周期由 gost 状态与分流开关唯一决定，与界面无关：
//
//	期望运行 = gost 正在运行 && 分流开关打开
//
// 所有启停都走 Reconcile —— 幂等，任何时机调用都安全。界面只读状态，
// 不触发启停。原来的实现把启动散在 doStart / watchSplit / 退出钩子里，
// 且以“自有进程是否存在”判断状态，导致 app 重启后误判未运行、
// 每 5 秒重启一次插件又撞端口，界面永久停在“将自动重启”。

// splitSupervisor 是插件的唯一管理者。
type splitSupervisor struct {
	app *app

	mu      sync.Mutex
	backoff time.Duration // 失败退避时长
	lastTry time.Time     // 上次尝试时间
	lastErr string
}

// 退避区间：插件起不来时不要每轮都重试（启动含 10s 就绪等待）。
const (
	splitBackoffMin = 1 * time.Second
	splitBackoffMax = 30 * time.Second
)

func newSplitSupervisor(a *app) *splitSupervisor {
	return &splitSupervisor{app: a, backoff: splitBackoffMin}
}

// Want 报告在当前 gost 状态与开关下，插件是否应该运行。
func (s *splitSupervisor) Want() bool {
	if !s.app.st.Get().Split {
		return false
	}
	st, _, _ := s.app.pm.State()
	return st == proc.Running
}

// Ensure 确保插件在运行且规则就绪，返回实际监听地址。
//
// 供“启动 gost 之前”调用：配置里的 split-geo 必须指向真实地址，
// 而地址可能因 8100 被占用而变。
func (s *splitSupervisor) Ensure(ctx context.Context) (string, error) {
	m := s.app.geoM
	st := s.app.st.Get()

	// 规则缺失时先下载（只在确实没有时打网络）
	if m.CountIP() <= 0 {
		if _, err := m.UpdateIPList(ctx, st.SplitIPListURL); err != nil {
			return m.Addr(), fmt.Errorf("更新 IP 列表失败: %w", err)
		}
	}
	if m.CountDomains() <= 0 {
		if _, err := m.UpdateDomainList(ctx, st.SplitDomainListURL); err != nil {
			return m.Addr(), fmt.Errorf("更新域名列表失败: %w", err)
		}
	}

	// 服务已在（自有或复用）→ 直接返回地址
	if m.Alive() {
		return m.Addr(), nil
	}
	if err := m.Start(ctx); err != nil {
		s.noteErr(err)
		return m.Addr(), err
	}
	if err := m.WaitReady(ctx, 15*time.Second); err != nil {
		s.noteErr(err)
		return m.Addr(), err
	}
	s.noteOK()
	return m.Addr(), nil
}

// Reconcile 把插件状态收敛到 Want() 期望值。幂等，事件与兜底轮询都调它。
func (s *splitSupervisor) Reconcile() {
	want := s.Want()
	alive := s.app.geoM.Alive()

	switch {
	case want && alive:
		s.noteOK() // 已在，无事可做
		return
	case want && !alive:
		if !s.allowRetry() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := s.Ensure(ctx); err != nil {
			s.noteErr(err)
			log.Printf("分流插件启动失败（%.0fs 后重试）: %v", s.currentBackoff().Seconds(), err)
		}
	case !want && alive:
		// 不该跑却还在：停掉自有进程（外部实例留给 ForceStop 场景）
		s.app.geoM.Stop()
	}
}

// StopIfUnwanted 在 gost 停止时调用：分流关掉后插件不应该继续占端口。
func (s *splitSupervisor) StopIfUnwanted() {
	if s.Want() {
		return
	}
	if err := s.app.geoM.ForceStop(); err != nil {
		log.Printf("停止分流插件: %v", err)
	}
}

// Err 返回最近一次失败原因（供界面显示）。
func (s *splitSupervisor) Err() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// allowRetry 按指数退避决定是否允许再次尝试启动。
func (s *splitSupervisor) allowRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastTry) >= s.backoff
}

func (s *splitSupervisor) currentBackoff() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backoff
}

// noteErr 记录失败并拉长退避。
func (s *splitSupervisor) noteErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastErr = err.Error()
	s.lastTry = time.Now()
	s.backoff *= 2
	if s.backoff > splitBackoffMax {
		s.backoff = splitBackoffMax
	}
}

// noteOK 清空失败状态并复位退避。
func (s *splitSupervisor) noteOK() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr == "" && s.backoff == splitBackoffMin {
		return
	}
	s.lastErr = ""
	s.backoff = splitBackoffMin
}

// Watch 启动兜底轮询：每 5 秒收敛一次。
//
// 事件（连接/断开/开关切换）已经会立即 Reconcile，这里只兜住漏网场景
// （崩溃、被外部 kill）。失败时按指数退避，不会每轮重试。
func (s *splitSupervisor) Watch() {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			if s.app.win == nil || s.app.win.IsDestroyed() {
				return
			}
			before := s.app.geoM.Alive()
			s.Reconcile()
			if s.app.geoM.Alive() != before {
				if w := s.app.win; w != nil {
					w.Invalidate()
				}
			}
		}
	}()
}
