package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/metrics"
	"mygost/internal/probe"
	"mygost/internal/proc"
	"mygost/internal/profiles"
)

// startMetrics 在连接成功后开始采样 gost 的 metrics 端点。
func (a *app) startMetrics(port int) {
	a.metricsPort = port
	if port <= 0 {
		a.mc.SetAddr("")
		return
	}
	a.mc.SetAddr("127.0.0.1:" + itoa(port))
	a.msp.Reset()
	a.metricsOK = false
	a.rates = metrics.Rates{}
	a.hasSessBase = false
	if a.stats != nil {
		a.stats.AddSession(todayKey())
	}
	go a.metricsLoop(port)
}

// stopMetrics 停止采样并清空显示。
func (a *app) stopMetrics() {
	a.metricsPort = 0
	a.mc.SetAddr("")
	a.metricsOK = false
	a.rates = metrics.Rates{}
	a.hasSessBase = false
	if a.stats != nil {
		if err := a.stats.Save(); err != nil {
			log.Printf("保存流量统计失败: %v", err)
		}
	}
}

// metricsLoop 每秒抓一次数据；gost 停止后自动退出。
func (a *app) metricsLoop(port int) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		if a.metricsPort != port {
			return // 端口变了或已停止
		}
		if st, _, _ := a.pm.State(); st != proc.Running {
			return
		}
		snap, err := a.mc.Scrape()
		a.win.Update(func() {
			if err != nil {
				a.metricsOK = false
				return
			}
			a.metricsOK = true
			a.rates = a.msp.Observe(snap)
			a.accumulate(snap)
		})
	}
}

// accumulate 把两次采样之间的增量累加到当天统计。
func (a *app) accumulate(snap metrics.Snapshot) {
	if a.stats == nil {
		return
	}
	if !a.hasSessBase {
		a.sessBase, a.hasSessBase = snap, true
		return
	}
	up := snap.Input - a.sessBase.Input
	down := snap.Output - a.sessBase.Output
	if up < 0 || down < 0 { // gost 重启：重取基准，忽略这次
		a.sessBase = snap
		return
	}
	dt := snap.At.Sub(a.sessBase.At).Seconds()
	a.sessBase = snap
	// 采样间隔异常（暂停/睡眠）时不计入时长，避免把长间隔算成流量时段
	if dt > 0 && dt < 10 {
		a.stats.Add(todayKey(), up, down, dt)
	}

	// 每 60 秒落盘一次，避免高频 IO
	if time.Since(a.statsSaved) >= time.Minute {
		a.statsSaved = time.Now()
		if err := a.stats.Save(); err != nil {
			log.Printf("保存流量统计失败: %v", err)
		}
	}
}

// todayKey 返回本地日期键 YYYY-MM-DD。
func todayKey() string { return time.Now().Format("2006-01-02") }

// testResults 按 profile ID 缓存测试结果（内联在配置行里显示）。
var _ = 0

// testResultOf 返回某配置的测试结果。
func (a *app) testResultOf(id string) ([]probe.Result, bool) {
	if a.testResByID == nil {
		return nil, false
	}
	r, ok := a.testResByID[id]
	return r, ok && len(r) > 0
}

// inlineTestResult 在配置行内渲染测试结果。
//
// 通过时列出每一项的名称与延迟（让用户知道测了什么），
// 失败时优先显示失败项的原因，宽字符允许换行最多三行。
func (a *app) inlineTestResult(c *ui.Context, results []probe.Result) {
	t := c.Theme()
	fails := make([]probe.Result, 0, len(results))
	passDetails := make([]string, 0, len(results))
	for _, r := range results {
		if r.OK {
			passDetails = append(passDetails, r.Name+" "+shortLatency(r.Latency))
		} else {
			fails = append(fails, r)
		}
	}

	dot := t.Success
	var text string
	if len(fails) == 0 {
		text = strings.Join(passDetails, " · ")
	} else {
		dot = t.Danger
		first := fails[0].Name + " " + fails[0].Detail
		if len(fails) == 1 {
			text = first
		} else {
			text = fmt.Sprintf(tr("%d 项失败 · %s", "%d failed · %s"), len(fails), first)
		}
	}

	ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Height(18).Center().Children(func() { statusDot(c, dot, dotSm) })
		ui.Text(c, text).FontSize(fsCaption).TextColor(t.TextMuted).MaxLines(3).Grow(1)
	})
}

// shortLatency 把延迟压成短形式（120ms / 1.2s）。
func shortLatency(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d < time.Second {
		return itoa(int(d.Round(time.Millisecond)/time.Millisecond)) + "ms"
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
}

// testProfile 测试指定配置（只走它的端口）。
func (a *app) testProfile(id string) {
	p := a.ps.Get(id)
	if p == nil || a.testing {
		return
	}
	a.win.Update(func() {
		a.testing = true
		if a.testResByID != nil {
			delete(a.testResByID, id)
		}
	})

	go func() {
		results := runProbe(p)
		a.win.Update(func() {
			a.testing = false
			if a.testResByID == nil {
				a.testResByID = map[string][]probe.Result{}
			}
			a.testResByID[id] = results
			// 结果保留 10 秒后自动恢复摘要
			go func() {
				time.Sleep(10 * time.Second)
				a.win.Update(func() { delete(a.testResByID, id) })
			}()
		})
	}()
}

// runConnectivityTest 测试当前选中配置。
func (a *app) runConnectivityTest() {
	p := a.currentProfile()
	if p == nil {
		a.toast(tr("没有选中的配置", "No profile selected"), "error")
		return
	}
	a.testProfile(p.ID)
}

// runProbe 执行一个配置的连通性测试（不依赖 UI 状态，便于测试）。
func runProbe(p *profiles.Profile) []probe.Result {
	var results []probe.Result

	switch p.Kind {
	case profiles.KindProxy:
		// ① 代理链路：显式通过本配置的本地端口
		if local, err := probe.ListenAddr(p.Listen); err == nil {
			results = append(results, probe.ViaHTTPProxy(local, probe.Target, probe.Timeout))
		} else {
			results = append(results, probe.Result{Name: "代理链路", Detail: err.Error()})
		}
		// ② 上游隧道 TCP 可达
		if p.UpEnabled && p.UpAddr != "" {
			results = append(results, probe.TCPDial("上游隧道", p.UpAddr, probe.Timeout))
		}
	case profiles.KindForward:
		if local, err := probe.ListenAddr(p.Listen); err == nil {
			results = append(results, probe.TCPDial("本地端口", local, probe.Timeout))
		}
		for i, target := range splitTargetList(p.Targets) {
			if i >= 3 {
				break
			}
			results = append(results, probe.TCPDial("转发目标", target, probe.Timeout))
		}
	}
	// ③ 直连基线：用于对比（不影响结论）
	results = append(results, probe.Direct(probe.Target, probe.Timeout))
	return results
}

// metricCell 是流量卡片里的一个数值单元。
func (a *app) metricCell(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(1).Children(func() {
		ui.Text(c, label).FontSize(fsCaption).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(fsEmphasis)
	})
}

// splitTargetList 把多行/逗号分隔的目标地址拆开。
func splitTargetList(s string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' }) {
		if v := strings.TrimSpace(line); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// itoa 简单整数转字符串。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// _ 保持 context 引用（runConnectivityTest 内可能用到超时控制）。
var _ = context.Background
