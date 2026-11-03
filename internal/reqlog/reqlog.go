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

// Log 保存最近的请求记录（环形缓冲）。
type Log struct {
	mu   sync.Mutex
	buf  []Entry
	next int
	full bool
	sids map[string]int // sid → 下标，用于把 route 行补进同一条
	on   bool
	now  func() time.Time
}

// New 创建记录器；capacity<=0 时用 500。
func New(capacity int) *Log {
	if capacity <= 0 {
		capacity = 500
	}
	return &Log{
		buf:  make([]Entry, capacity),
		sids: make(map[string]int),
		on:   true,
		now:  time.Now,
	}
}

// SetEnabled 开关解析：关闭后不再产生记录（原始日志不受影响）。
func (l *Log) SetEnabled(on bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.on = on
	l.mu.Unlock()
}

// Enabled 报告解析是否开启。
func (l *Log) Enabled() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.on
}

// Entries 返回最近的记录（时间升序）。max<=0 表示全部。
func (l *Log) Entries(max int) []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	if l.full {
		out = append(out, l.buf[l.next:]...)
	}
	out = append(out, l.buf[:l.next]...)
	if max > 0 && len(out) > max {
		out = out[len(out)-max:]
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
func (l *Log) Feed(line string) {
	if l == nil || !l.Enabled() {
		return
	}
	msg, sid := extract(line)
	if msg == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.applyLocked(msg, sid)
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
		return
	}
	e := Entry{At: l.now(), Target: target, SID: sid, Decision: dec}
	idx := l.next
	l.buf[idx] = e
	if sid != "" {
		l.sids[sid] = idx
	}
	l.next++
	if l.next >= len(l.buf) {
		l.next = 0
		l.full = true
	}
}

// setNodeLocked 给已有记录补节点。
func (l *Log) setNodeLocked(sid, node string) {
	if idx, ok := l.sids[sid]; ok && sid != "" {
		l.buf[idx].Node = node
	}
}

// setErrLocked 给已有记录（或新建）补失败原因。
func (l *Log) setErrLocked(sid, reason string) {
	if idx, ok := l.sids[sid]; ok && sid != "" {
		l.buf[idx].Err = reason
		if l.buf[idx].Decision == Unknown {
			l.buf[idx].Decision = Proxied
		}
		return
	}
	e := Entry{At: l.now(), SID: sid, Err: reason, Decision: Proxied}
	idx := l.next
	l.buf[idx] = e
	if sid != "" {
		l.sids[sid] = idx
	}
	l.next++
	if l.next >= len(l.buf) {
		l.next = 0
		l.full = true
	}
}
