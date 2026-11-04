package main

import (
	"path/filepath"
	"time"

	"mygost/internal/geo"
	"mygost/internal/proc"
)

// ─── 视图状态：重操作与渲染分离 ───────────────────────────────────
//
// 渲染函数每帧都会跑，滚动时更是每秒几十次。凡是会 fork 进程
// （pgrep/lsof）、发起网络连接（端口探活）、或读大文件的操作都不能
// 待在 view 里——否则设置页滚起来就是幻灯片。
//
// 做法：后台 goroutine 定时探测，结果写进 viewModel；view 只读内存
// 字段，零 syscall。

// refreshInterval 是后台探测周期。2 秒足以让状态看起来实时，
// 又不会让 pgrep/dial 的代价落到渲染上。
const refreshInterval = 2 * time.Second

// probeViewModel 执行一次探测，返回快照。纯计算，不碰 UI 状态，
// 因此可安全地在后台 goroutine 里跑。
func (a *app) probeViewModel() viewModel {
	var vm viewModel

	// 插件端口探活：TCP dial 带 syscall 与超时开销
	vm.splitAlive = a.geoM.Running() && portOpen(a.geoM.Addr())

	// 残留进程：pgrep 会 fork 一个进程，是这几项里最贵的
	if all := proc.FindByConfig(filepath.Join(dataDir, "profiles")); len(all) > 0 {
		cur := a.pm.Pid()
		for _, p := range all {
			if p != cur {
				vm.leftovers = append(vm.leftovers, p)
			}
		}
	}

	// 规则规模：列表文件 11 万行，geo.Manager 内部按 mtime 缓存
	vm.domainCount = a.geoM.CountDomains()
	vm.ipCount = a.geoM.CountIP()
	vm.rulesReady = a.geoM.Ready()
	vm.rulesAt = latestRuleTime(a.geoM)

	return vm
}

// refreshViewModel 同步刷新一次（UI 线程调用，首帧与事件后使用）。
func (a *app) refreshViewModel() { a.vm = a.probeViewModel() }

// watchViewModel 启动后台探测循环。没有窗口时（测试、无头渲染）不启动，
// 由测试直接调 refreshViewModel。
func (a *app) watchViewModel() {
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for range t.C {
			if a.win == nil || a.win.IsDestroyed() {
				return
			}
			vm := a.probeViewModel()
			a.win.Update(func() {
				a.vm = vm
				a.win.Invalidate()
			})
		}
	}()
}

// latestRuleTime 取两份规则文件里较新的修改时间。
func latestRuleTime(m *geo.Manager) time.Time {
	ip := m.UpdateAt(m.IPListPath())
	dm := m.UpdateAt(m.DomainListPath())
	if dm.After(ip) {
		return dm
	}
	return ip
}
