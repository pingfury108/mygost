// Package reqlog 从 gost 的 debug 日志里解析"每个请求走了哪里"。
//
// gost 在 log.level=debug 时输出（实测）：
//
//	{"msg":"bypass: 192.168.1.10:8080, whitelist: false"}          → 判定直连
//	{"msg":"proxy: example.com:80, whitelist: false"}              → 判定走代理
//	{"msg":"route(retry=0) 192.168.1.10:8080"}                     → 实际直连
//	{"msg":"route(retry=0) node-0@1.2.3.4:8081 > example.com:80"}  → 实际走上游
//	{"msg":"route(retry=0) dial tcp ...: connection refused"}      → 失败
//
// 解析器容错：认不出的行直接忽略，不影响原始日志。
package reqlog

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Decision 是请求的去向。
type Decision int

const (
	Unknown Decision = iota
	Direct           // 直连（命中 bypass）
	Proxied          // 走上游转发链
)

// String 返回中文描述。
func (d Decision) String() string {
	switch d {
	case Direct:
		return "直连"
	case Proxied:
		return "代理"
	default:
		return "未知"
	}
}

// Entry 是一条请求记录。
type Entry struct {
	At       time.Time
	Target   string // host:port
	SID      string // gost 的会话 ID，用于把多行归并成一条
	Decision Decision
	Node     string        // 走代理时的节点（node-0@1.2.3.4:8081）
	Latency  time.Duration // 完成耗时
	Err      string        // 失败原因
}

// Log 保存最近的请求记录（环形缓冲）并落盘。
type Log struct {
	mu    sync.Mutex
	buf   []Entry
	next  int
	full  bool
	sids  map[string]int // sid → 下标，用于把 route 行补进同一条
	revs  []uint64       // 每个槽位最后修改的序号
	rev   uint64         // 单调递增的修改序号
	done  uint64         // Flush 已处理的序号
	on    atomic.Bool
	now   func() time.Time
	store *Store // 持久化；nil 表示不落盘
}

// New 创建记录器；capacity<=0 时用 500。
func New(capacity int) *Log {
	if capacity <= 0 {
		capacity = 500
	}
	l := &Log{
		buf:  make([]Entry, capacity),
		sids: make(map[string]int),
		revs: make([]uint64, capacity),
		now:  time.Now,
	}
	l.on.Store(true)
	return l
}

// SetStore 挂上持久化后端；nil 表示只留内存。
func (l *Log) SetStore(s *Store) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.store = s
	l.mu.Unlock()
}

// Flush 把当前记录写入磁盘（退出、切天、定时器调用）。
func (l *Log) Flush() {
	if l == nil {
		return
	}
	s := l.storeRef()
	if s == nil {
		return
	}
	entries := l.changed()
	for _, e := range entries {
		s.Append(e)
	}
	s.Flush()
}

// changed 返回上次 Flush 之后新建或更新过的记录（时间升序）。
//
// 记录会在内存里被后续日志行补全（node/err），所以不能只取“最新一条”；
// 但也不能每 500ms 把整个环形缓冲重新格式化查重。rev 让 Flush 只碰
// 真正变化过的记录——空闲或低速时几乎是空操作。
func (l *Log) changed() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rev == l.done {
		return nil
	}
	total := l.next
	if l.full {
		total = len(l.buf)
	}
	out := make([]Entry, 0, total)
	maxRev := l.done
	start := l.next - total
	if start < 0 {
		start += len(l.buf)
	}
	for i := 0; i < total; i++ {
		idx := start + i
		if idx >= len(l.buf) {
			idx -= len(l.buf)
		}
		if r := l.revs[idx]; r > l.done {
			out = append(out, l.buf[idx])
			if r > maxRev {
				maxRev = r
			}
		}
	}
	l.done = maxRev
	return out
}

// Store 返回持久化后端（可能为 nil）。
func (l *Log) Store() *Store {
	if l == nil {
		return nil
	}
	return l.storeRef()
}

func (l *Log) storeRef() *Store {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.store
}

// SetEnabled 开关解析：关闭后不再产生记录（原始日志不受影响）。
func (l *Log) SetEnabled(on bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.on.Store(on)
	l.mu.Unlock()
}

// Enabled 报告解析是否开启。
func (l *Log) Enabled() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.on.Load()
}

// Entries 返回最近的记录（时间升序）。max<=0 表示全部。
func (l *Log) Entries(max int) []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	total := l.next
	if l.full {
		total = len(l.buf)
	}
	if total == 0 {
		return nil
	}
	n := total
	if max > 0 && n > max {
		n = max
	}
	start := l.next - n
	if start < 0 {
		start += len(l.buf)
	}
	out := make([]Entry, n)
	copied := copy(out, l.buf[start:])
	if copied < n {
		copy(out[copied:], l.buf[:n-copied])
	}
	return out
}

// Clear 清空记录。
func (l *Log) Clear() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next, l.full = 0, false
	l.sids = make(map[string]int)
}

// Len 返回当前记录数。
func (l *Log) Len() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.full {
		return len(l.buf)
	}
	return l.next
}

// Feed 送入一行原始日志（proc 的时间戳前缀已去除）。
// 返回 true 表示这行参与了请求归并，调用方可据此刷新视图；
// 高频路径不再额外查询 Len/最后一条记录。
func (l *Log) Feed(line string) bool {
	if l == nil || !l.on.Load() {
		return false
	}
	msg, sid := extract(line)
	if msg == "" {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.applyLocked(msg, sid)
	return true
}

// applyLocked 根据一条 msg 更新记录。
func (l *Log) applyLocked(msg, sid string) {
	switch {
	case strings.HasPrefix(msg, "bypass: "):
		target := trimAfter(msg[len("bypass: "):])
		l.upsertLocked(sid, target, Direct)
	case strings.HasPrefix(msg, "proxy: "):
		target := trimAfter(msg[len("proxy: "):])
		l.upsertLocked(sid, target, Proxied)
	case strings.HasPrefix(msg, "route(retry="):
		l.applyRouteLocked(msg, sid)
	}
}

// applyRouteLocked 处理 route 行：带 ">" 表示经节点，否则直连。
func (l *Log) applyRouteLocked(msg, sid string) {
	// route(retry=0) node-0@1.2.3.4:8081 > example.com:80
	body := msg
	if i := strings.Index(body, ") "); i >= 0 {
		body = body[i+2:]
	}
	if body == "" {
		return
	}

	// 失败：route(retry=0) dial tcp ...: connection refused
	if strings.HasPrefix(body, "dial ") || strings.Contains(body, ": connection ") {
		l.setErrLocked(sid, shortErr(body))
		return
	}

	dec, node, target := Proxied, "", body
	if k := strings.LastIndex(body, " > "); k >= 0 {
		// 多级链：node-0@a > node-1@b > target，取最后一段为目标、其余为链路
		node = body[:k]
		target = body[k+3:]
	} else {
		dec = Direct // 无节点前缀 = 本机直连
	}
	l.upsertLocked(sid, target, dec)
	if node != "" {
		l.setNodeLocked(sid, node)
	}
}

// extract 从一行 JSON 日志里取出 msg 与 sid。
func extract(line string) (msg, sid string) {
	// 只有这三类日志参与请求归并；先做廉价判断，
	// 避免对每条 debug 日志（约 11 行/请求）都做一次 json.Unmarshal。
	if !strings.Contains(line, `"msg":"bypass: `) &&
		!strings.Contains(line, `"msg":"proxy: `) &&
		!strings.Contains(line, `"msg":"route(retry=`) {
		return "", ""
	}
	// gost 的日志是 encoding/json 生成的紧凑单行 JSON。
	// 直接取字符串字段可省掉每条匹配日志的反射解码；格式异常时再回退。
	if m, ok := rawJSONString(line, "msg"); ok {
		s, _ := rawJSONString(line, "sid")
		return m, s
	}
	i := strings.Index(line, "{")
	if i < 0 {
		return "", ""
	}
	var payload struct {
		Msg string `json:"msg"`
		SID string `json:"sid"`
	}
	if err := json.Unmarshal([]byte(line[i:]), &payload); err != nil {
		return "", ""
	}
	return payload.Msg, payload.SID
}

// rawJSONString 从紧凑 JSON 行里取出字符串字段，不处理转义（调用方遇反斜杠回退）。
func rawJSONString(line, key string) (string, bool) {
	marker := `"` + key + `":"`
	i := strings.Index(line, marker)
	if i < 0 {
		return "", false
	}
	start := i + len(marker)
	for j := start; j < len(line); j++ {
		switch line[j] {
		case '\\':
			return "", false // 含转义，交给 encoding/json
		case '"':
			return line[start:j], true
		}
	}
	return "", false
}

// trimAfter 去掉 ", whitelist: false" 之类的后缀。
func trimAfter(s string) string {
	if i := strings.Index(s, ","); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// shortErr 把 dial 错误压短。
func shortErr(s string) string {
	switch {
	case strings.Contains(s, "connection refused"):
		return "连接被拒绝"
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "deadline exceeded"):
		return "超时"
	case strings.Contains(s, "no such host"):
		return "域名解析失败"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// upsertLocked 写入或更新一条记录。
func (l *Log) upsertLocked(sid, target string, dec Decision) {
	if target == "" {
		return
	}
	if idx, ok := l.sids[sid]; ok && sid != "" {
		e := &l.buf[idx]
		e.Target = target
		if dec != Unknown {
			e.Decision = dec
		}
		l.touchLocked(idx)
		return
	}
	e := Entry{At: l.now(), Target: target, SID: sid, Decision: dec}
	idx := l.nextSlotLocked()
	l.buf[idx] = e
	l.touchLocked(idx)
	if sid != "" {
		l.sids[sid] = idx
	}
}

// setNodeLocked 给已有记录补节点。
func (l *Log) setNodeLocked(sid, node string) {
	if idx, ok := l.sids[sid]; ok && sid != "" {
		l.buf[idx].Node = node
		l.touchLocked(idx)
	}
}

// setErrLocked 给已有记录（或新建）补失败原因。
func (l *Log) setErrLocked(sid, reason string) {
	if idx, ok := l.sids[sid]; ok && sid != "" {
		l.buf[idx].Err = reason
		if l.buf[idx].Decision == Unknown {
			l.buf[idx].Decision = Proxied
		}
		l.touchLocked(idx)
		return
	}
	e := Entry{At: l.now(), SID: sid, Err: reason, Decision: Proxied}
	idx := l.nextSlotLocked()
	l.buf[idx] = e
	l.touchLocked(idx)
	if sid != "" {
		l.sids[sid] = idx
	}
}

// touchLocked 标记槽位在本次 Flush 后发生了变化。
func (l *Log) touchLocked(idx int) {
	l.rev++
	l.revs[idx] = l.rev
}

// nextSlotLocked 返回下一个写入槽位，并清掉即将被覆盖记录的 sid 索引。
//
// sids 只为「把同一次请求的多行日志归并成一条」服务；槽位被复用时若不删除
// 旧 sid，map 会随请求数无限增长（长跑几天可达几万项）。
func (l *Log) nextSlotLocked() int {
	idx := l.next
	if old := l.buf[idx].SID; old != "" {
		if mapped, ok := l.sids[old]; ok && mapped == idx {
			delete(l.sids, old)
		}
	}
	l.next++
	if l.next >= len(l.buf) {
		l.next = 0
		l.full = true
	}
	return idx
}
