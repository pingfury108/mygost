// Package metrics：按天持久化流量统计。
package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Retention 是历史数据保留天数，超出的记录会被丢弃。
const Retention = 31

// Day 是一天的累计流量。
type Day struct {
	Date     string  `json:"date"` // YYYY-MM-DD（本地时区）
	Up       float64 `json:"up"`   // 输入字节
	Down     float64 `json:"down"` // 输出字节
	Sessions int     `json:"sessions"`
	Seconds  float64 `json:"seconds"` // 累计运行秒数
}

// Store 按天累加并落盘到 stats.json。
type Store struct {
	mu    sync.Mutex
	path  string
	days  map[string]*Day
	dirty bool
}

// LoadStore 读取 dir/stats.json；不存在时返回空存储。
func LoadStore(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "stats.json"), days: map[string]*Day{}}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	var payload struct {
		Days []Day `json:"days"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return s, nil // 文件损坏：重新开始，不阻塞应用
	}
	for i := range payload.Days {
		d := payload.Days[i]
		if d.Date == "" {
			continue
		}
		cp := d
		s.days[d.Date] = &cp
	}
	s.pruneLocked(time.Now())
	return s, nil
}

// today 返回本地时区的日期键。
func today(now time.Time) string { return now.Format("2006-01-02") }

// Add 把一段时间内的流量累加到指定的日期。
func (s *Store) Add(date string, up, down float64, seconds float64) {
	if s == nil || date == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.days[date]
	if d == nil {
		d = &Day{Date: date}
		s.days[date] = d
	}
	if up > 0 {
		d.Up += up
	}
	if down > 0 {
		d.Down += down
	}
	if seconds > 0 {
		d.Seconds += seconds
	}
	s.dirty = true
}

// AddSession 记一次会话开始。
func (s *Store) AddSession(date string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.days[date]
	if d == nil {
		d = &Day{Date: date}
		s.days[date] = d
	}
	d.Sessions++
	s.dirty = true
}

// Today 返回今天的记录（不存在时为零值）。
func (s *Store) Today() Day {
	if s == nil {
		return Day{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.days[today(time.Now())]; d != nil {
		return *d
	}
	return Day{Date: today(time.Now())}
}

// Recent 返回最近 n 天的记录，按日期升序（缺失的日期不补）。
func (s *Store) Recent(n int) []Day {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 {
		n = Retention
	}
	out := make([]Day, 0, n)
	for _, d := range s.days {
		out = append(out, *d)
	}
	// 按日期升序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Date < out[j-1].Date; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// Total 返回所有保留记录的合计。
func (s *Store) Total() Day {
	if s == nil {
		return Day{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var t Day
	t.Date = "合计"
	for _, d := range s.days {
		t.Up += d.Up
		t.Down += d.Down
		t.Sessions += d.Sessions
		t.Seconds += d.Seconds
	}
	return t
}

// Save 落盘（仅在 dirty 时真正写）。
func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	s.pruneLocked(time.Now())

	payload := struct {
		Days []Day `json:"days"`
	}{}
	for _, d := range s.days {
		payload.Days = append(payload.Days, *d)
	}
	for i := 1; i < len(payload.Days); i++ {
		for j := i; j > 0 && payload.Days[j].Date < payload.Days[j-1].Date; j-- {
			payload.Days[j], payload.Days[j-1] = payload.Days[j-1], payload.Days[j]
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.dirty = false
	return nil
}

// pruneLocked 丢弃超过 Retention 天的记录。
func (s *Store) pruneLocked(now time.Time) {
	cutoff := now.AddDate(0, 0, -Retention).Format("2006-01-02")
	for k := range s.days {
		if k < cutoff {
			delete(s.days, k)
			s.dirty = true
		}
	}
}

// Prune 显式清理过期数据（导出给测试与启动时调用）。
func (s *Store) Prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
}

// Len 返回当前保留的天数。
func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.days)
}
