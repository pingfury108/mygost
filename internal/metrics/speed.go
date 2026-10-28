package metrics

import (
	"strconv"
	"strings"
	"sync"
)

// Rates 是一段时间内的速率与连接状态。
type Rates struct {
	Up        float64 // 字节/秒（输入方向）
	Down      float64 // 字节/秒（输出方向）
	Requests  float64 // 请求/秒
	TotalUp   float64 // 本次会话累计输入字节
	TotalDown float64 // 本次会话累计输出字节
	TotalReqs float64
	Errors    float64
}

// Speed 由连续采样求速率，处理计数器归零（gost 重启）。
type Speed struct {
	mu      sync.Mutex
	prev    *Snapshot
	base    Snapshot // 会话基准，用于累计值
	hasBase bool

	// 平滑：最近若干次的速率
	upHist, downHist []float64
	window           int
}

// NewSpeed 创建速率统计；window 是滑动平均的样本数（<=0 时用 5）。
func NewSpeed(window int) *Speed {
	if window <= 0 {
		window = 5
	}
	return &Speed{window: window}
}

// Reset 在会话开始时调用，重置基准。
func (s *Speed) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prev = nil
	s.hasBase = false
	s.upHist = nil
	s.downHist = nil
}

// Observe 记录一次采样并返回当前速率。
//
// 计数器回退（gost 重启）时重新取基准，不产生负速度。
func (s *Speed) Observe(snap Snapshot) Rates {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasBase {
		s.base = snap
		s.hasBase = true
	}

	var r Rates
	r.TotalUp = snap.Input - s.base.Input
	r.TotalDown = snap.Output - s.base.Output
	r.TotalReqs = snap.Reqs - s.base.Reqs
	r.Errors = snap.Errors

	if r.TotalUp < 0 || r.TotalDown < 0 { // 计数器归零：重置基准
		s.base = snap
		s.prev = nil
		return Rates{Errors: snap.Errors}
	}

	p := s.prev
	s.prev = &snap
	if p == nil {
		return r // 首次采样没有速率
	}
	dt := snap.At.Sub(p.At).Seconds()
	if dt <= 0 {
		return r
	}
	up := (snap.Input - p.Input) / dt
	down := (snap.Output - p.Output) / dt
	if up < 0 || down < 0 { // 退回：忽略本次速率
		return r
	}
	r.Up = s.push(&s.upHist, up)
	r.Down = s.push(&s.downHist, down)
	r.Requests = (snap.Reqs - p.Reqs) / dt
	return r
}

// push 追加样本并返回滑动平均。
func (s *Speed) push(hist *[]float64, v float64) float64 {
	*hist = append(*hist, v)
	if len(*hist) > s.window {
		*hist = (*hist)[len(*hist)-s.window:]
	}
	sum := 0.0
	for _, x := range *hist {
		sum += x
	}
	return sum / float64(len(*hist))
}

// FormatBytes 把字节数格式化成人读形式。
func FormatBytes(v float64) string {
	const unit = 1024.0
	if v < unit {
		return itoa(v) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	x := v
	for _, u := range units {
		x /= unit
		if x < unit {
			return trimFloat(x) + " " + u
		}
	}
	return trimFloat(x) + " PB"
}

// FormatRate 把字节/秒格式化。
func FormatRate(v float64) string { return FormatBytes(v) + "/s" }

func trimFloat(v float64) string {
	if v >= 100 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

func itoa(v float64) string { return strconv.FormatFloat(v, 'f', 0, 64) }
