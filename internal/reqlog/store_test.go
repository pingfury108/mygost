package reqlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// 写入后能原样读回。
func TestStoreRoundTrip(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	s.Append(Entry{At: now, Target: "a.com:443", SID: "s1", Decision: Direct})
	s.Append(Entry{At: now, Target: "b.com:443", SID: "s2", Decision: Proxied,
		Node: "node-0@1.2.3.4:8081", Err: "超时"})
	s.Flush()

	got, err := s.Load(now.Format(dayLayout))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("读回 %d 条，want 2: %+v", len(got), got)
	}
	if got[0].Target != "a.com:443" || got[1].Node != "node-0@1.2.3.4:8081" {
		t.Errorf("内容不符: %+v", got)
	}
}

// 同 key 更新（node/err 后补）应覆盖而不是产生两条。
func TestStoreUpdatesInPlace(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	day := now.Format(dayLayout)

	s.Append(Entry{At: now, Target: "x.com:443", SID: "k", Decision: Proxied})
	s.Append(Entry{At: now, Target: "x.com:443", SID: "k", Decision: Proxied, Node: "node-0@h:1"})
	s.Flush()

	got, _ := s.Load(day)
	if len(got) != 1 {
		t.Fatalf("同 key 应归一，实际 %d 条", len(got))
	}
	if got[0].Node != "node-0@h:1" {
		t.Errorf("应取最后一行，得到 %+v", got[0])
	}
}

// 内容没变时不重复追加。
func TestStoreSkipsIdentical(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	day := now.Format(dayLayout)
	e := Entry{At: now, Target: "dup.com:443", SID: "d", Decision: Direct}
	for i := 0; i < 5; i++ {
		s.Append(e)
	}
	s.Flush()

	path := filepath.Join(s.dir, day+".jsonl")
	if n := countLines(path); n != 1 {
		t.Errorf("重复写入应去重，文件行数 = %d, want 1", n)
	}
}

// 超过保留期的文件被清理，保留 7 天。
func TestStorePruneKeepsRetention(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	for i := 0; i < 12; i++ {
		day := now.AddDate(0, 0, -i)
		os.WriteFile(filepath.Join(s.dir, day.Format(dayLayout)+".jsonl"), []byte("{}\n"), 0o644)
	}
	s.Prune()

	left, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != Retention {
		t.Errorf("保留 %d 个文件，want %d", len(left), Retention)
	}
	// 最旧的应被删掉
	oldest := now.AddDate(0, 0, -(Retention)).Format(dayLayout)
	if _, err := os.Stat(filepath.Join(s.dir, oldest+".jsonl")); !os.IsNotExist(err) {
		t.Error("超出保留期的文件应被删除")
	}
}

// 单日超过上限后不再写入。
func TestStoreDailyLimit(t *testing.T) {
	s := newStore(t)
	s.limit = 3 // 测试用小上限
	now := time.Now()
	for i := 0; i < 10; i++ {
		s.Append(Entry{At: now, Target: "x.com", SID: string(rune('a' + i)), Decision: Direct})
	}
	s.Flush()

	got, _ := s.Load(now.Format(dayLayout))
	if len(got) != 3 {
		t.Errorf("超过每日上限应停止写入，实际 %d 条", len(got))
	}
}

// 跨天自动切换文件。
func TestStoreRotatesByDay(t *testing.T) {
	s := newStore(t)
	d1 := time.Now().AddDate(0, 0, -1)
	d2 := time.Now()
	s.Append(Entry{At: d1, Target: "a.com", SID: "1", Decision: Direct})
	s.Append(Entry{At: d2, Target: "b.com", SID: "2", Decision: Direct})
	s.Flush()

	for _, d := range []time.Time{d1, d2} {
		got, _ := s.Load(d.Format(dayLayout))
		if len(got) != 1 {
			t.Errorf("%s 应有 1 条，实际 %d", d.Format(dayLayout), len(got))
		}
	}
}

// Days 返回可用日期，新→旧，最多 Retention 个。
func TestStoreDaysOrder(t *testing.T) {
	s := newStore(t)
	now := time.Now()
	for i := 0; i < 3; i++ {
		s.Append(Entry{At: now.AddDate(0, 0, -i), Target: "x.com",
			SID: string(rune('a' + i)), Decision: Direct})
	}
	s.Flush()

	days := s.Days()
	if len(days) != 3 {
		t.Fatalf("Days = %v", days)
	}
	for i := 1; i < len(days); i++ {
		if days[i-1] < days[i] {
			t.Errorf("日期应为新→旧: %v", days)
		}
	}
}

// 端到端：真实 gost 日志行 → 解析 → 落盘 → 读回。
func TestFeedPersistsToDisk(t *testing.T) {
	s := newStore(t)
	l := New(50)
	l.SetStore(s)

	now := time.Now()
	l.now = func() time.Time { return now }
	// 走代理并补节点
	l.Feed(`{"msg":"proxy 149.129.41.181:8081","sid":"p1"}`)
	l.Feed(`{"msg":"route(retry=0) node-0@149.129.41.181:8081 > gateway.icloud.com:443","sid":"p1"}`)
	// 直连
	l.Feed(`{"msg":"bypass: www.baidu.com:443, whitelist: false","sid":"d1"}`)
	l.Feed(`{"msg":"route(retry=0) www.baidu.com:443","sid":"d1"}`)
	// 失败
	l.Feed(`{"msg":"proxy 149.129.41.181:8081","sid":"e1"}`)
	l.Feed(`{"msg":"route(retry=0) dial tcp 149.129.41.181:8081: connect: connection refused","sid":"e1"}`)

	l.Flush()

	got, err := s.Load(now.Format(dayLayout))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("读到 %d 条，want 3: %+v", len(got), got)
	}
	byTarget := map[string]Entry{}
	for _, e := range got {
		byTarget[e.Target] = e
	}
	if e, ok := byTarget["gateway.icloud.com:443"]; !ok || e.Decision != Proxied || e.Node == "" {
		t.Errorf("代理记录不符: %+v", e)
	}
	if e, ok := byTarget["www.baidu.com:443"]; !ok || e.Decision != Direct {
		t.Errorf("直连记录不符: %+v", e)
	}
}
