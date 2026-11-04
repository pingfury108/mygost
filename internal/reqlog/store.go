package reqlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── 请求记录持久化 ────────────────────────────────────────────────
//
// 按天一个 JSONL 文件（requests/2026-11-04.jsonl），追加写。
//
// 为什么不学 stats.json 做全量重写：debug 日志一天可达数万条，
// 全量重写的成本随记录数线性增长；追加是 O(1)，保留策略也退化成
// “删过期文件”。
//
// 同一条请求会分多行日志到达（bypass → route → node/err），解析时
// 是就地更新，所以这里允许同一 key 出现多行：读取时以最后一行为准。

// Retention 是请求记录保留天数。
const Retention = 7

// DailyLimit 是单日最多记录条数。debug 日志可能很密，
// 超过后当天不再写入（避免无限膨胀），界面会提示。
const DailyLimit = 50000

// 落盘参数：攒够一批或闲置超过 flushEvery 才写，避免高频 fsync。
const (
	flushBatch = 32
	flushEvery = 2 * time.Second
)

const dayLayout = "2006-01-02"

// storeLine 是 JSONL 里的一行。
type storeLine struct {
	Key      string    `json:"key"`
	At       time.Time `json:"at"`
	Target   string    `json:"target,omitempty"`
	Decision int       `json:"decision"`
	Node     string    `json:"node,omitempty"`
	Err      string    `json:"err,omitempty"`
}

// Store 把请求记录按天追加到磁盘。
type Store struct {
	mu     sync.Mutex
	dir    string
	f      *os.File
	w      *bufio.Writer
	day    string
	count  int       // 当天已写条数
	lastAt time.Time // 上次落盘
	seen   map[string]string
	// written 记录已落盘的内容指纹，内容变化才追加新行
	written map[string]string
	limit   int
}

// OpenStore 打开（或创建）记录目录，并清理过期文件。
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, seen: map[string]string{}, written: map[string]string{}, limit: DailyLimit}
	s.Prune()
	return s, nil
}

// Prune 删除超过保留期的文件（保留含今天在内的 Retention 天）。
func (s *Store) Prune() {
	if s == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -(Retention - 1)).Format(dayLayout)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day := strings.TrimSuffix(name, ".jsonl")
		if day < cutoff {
			os.Remove(filepath.Join(s.dir, name))
		}
	}
}

// Append 追加或更新一条记录。同 key 内容变化时会追加新行（读取取最后一行）。
func (s *Store) Append(e Entry) {
	if s == nil {
		return
	}
	key := entryKey(e)
	line := storeLine{
		Key: key, At: e.At, Target: e.Target,
		Decision: int(e.Decision), Node: e.Node, Err: e.Err,
	}
	fp := fmt.Sprintf("%s|%d|%s|%s", e.Target, e.Decision, e.Node, e.Err)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.written[key] == fp {
		return // 内容未变，不重复写
	}
	b, err := json.Marshal(line)
	if err != nil {
		return
	}
	if err := s.rotateLocked(e.At); err != nil {
		return
	}
	if s.count >= s.limit {
		return // 当天达上限：静默丢弃，界面按记录数提示
	}
	if _, err := s.w.Write(b); err != nil {
		return
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return
	}
	s.written[key] = fp
	s.count++
	if s.count%flushBatch == 0 || time.Since(s.lastAt) > flushEvery {
		s.flushLocked()
	}
}

// Flush 把缓冲写入磁盘（退出、切天、批量结束时调用）。
func (s *Store) Flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
}

// Close 落盘并关闭文件。
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked()
	if s.f != nil {
		err := s.f.Close()
		s.f, s.w = nil, nil
		return err
	}
	return nil
}

func (s *Store) flushLocked() {
	if s.w != nil {
		_ = s.w.Flush()
	}
	if s.f != nil {
		_ = s.f.Sync()
	}
	s.lastAt = time.Now()
}

// rotateLocked 确保写入的是 e.At 那一天的文件。
func (s *Store) rotateLocked(at time.Time) error {
	day := at.Format(dayLayout)
	if s.day == day && s.w != nil {
		return nil
	}
	s.flushLocked()
	if s.f != nil {
		_ = s.f.Close()
		s.f, s.w = nil, nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, day+".jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.f, s.w, s.day = f, bufio.NewWriterSize(f, 64*1024), day
	s.written = map[string]string{}
	s.count = countLines(filepath.Join(s.dir, day+".jsonl"))
	return nil
}

// Load 读取某一天的记录（同 key 以最后一行为准）。
func (s *Store) Load(day string) ([]Entry, error) {
	if s == nil {
		return nil, nil
	}
	s.Flush() // 保证当天的缓冲已落盘
	f, err := os.Open(filepath.Join(s.dir, day+".jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type item struct {
		e   Entry
		idx int
	}
	byKey := map[string]item{}
	next := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var ln storeLine
		if json.Unmarshal(sc.Bytes(), &ln) != nil {
			continue
		}
		e := Entry{
			At: ln.At, Target: ln.Target, SID: ln.Key,
			Decision: Decision(ln.Decision), Node: ln.Node, Err: ln.Err,
		}
		if it, ok := byKey[ln.Key]; ok {
			byKey[ln.Key] = item{e: e, idx: it.idx}
		} else {
			byKey[ln.Key] = item{e: e, idx: next}
			next++
		}
	}
	out := make([]Entry, next)
	for _, it := range byKey {
		out[it.idx] = it.e
	}
	return out, nil
}

// Dir 返回存储目录（诊断用）。
func (s *Store) Dir() string {
	if s == nil {
		return ""
	}
	return s.dir
}

// Days 返回可用的日期（新→旧），最多 Retention 天。
func (s *Store) Days() []string {
	if s == nil {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var days []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		days = append(days, strings.TrimSuffix(name, ".jsonl"))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	if len(days) > Retention {
		days = days[:Retention]
	}
	return days
}

// DayCount 返回某天已记录的条数（用于判断是否达到上限）。
func (s *Store) DayCount(day string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	if s.day == day {
		n := s.count
		s.mu.Unlock()
		return n
	}
	s.mu.Unlock()
	return countLines(filepath.Join(s.dir, day+".jsonl"))
}

// Limit 返回单日上限。
func (s *Store) Limit() int {
	if s == nil {
		return DailyLimit
	}
	return s.limit
}

// entryKey 生成去重键：有 sid 用 sid，否则用时间+目标。
func entryKey(e Entry) string {
	if e.SID != "" {
		return e.SID
	}
	return e.At.Format(time.RFC3339Nano) + "|" + e.Target
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n, sc := 0, bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			n++
		}
	}
	return n
}
