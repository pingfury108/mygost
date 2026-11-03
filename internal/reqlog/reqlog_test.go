package reqlog

import (
	"strings"
	"testing"
)

// 真实日志样本（来自实测输出）。
const (
	lineBypass = `{"time":"2026-11-03T16:23:16.077+08:00","level":"debug","msg":"bypass: 192.168.1.10:8080, whitelist: false","kind":"bypass","bypass":"priv","sid":"aaa"}`
	lineProxy  = `{"time":"2026-11-03T16:23:19.097+08:00","level":"debug","msg":"proxy: example.com:80, whitelist: false","kind":"bypass","bypass":"priv","sid":"bbb"}`
	lineRouteD = `{"time":"2026-11-03T16:23:16.079+08:00","level":"debug","msg":"route(retry=0) 192.168.1.10:8080","kind":"handler","sid":"aaa"}`
	lineRouteP = `{"time":"2026-11-03T16:23:19.098+08:00","level":"debug","msg":"route(retry=0) node-0@127.0.0.1:59999 > example.com:80","kind":"handler","sid":"bbb"}`
	lineDialEr = `{"time":"2026-11-03T16:23:19.101+08:00","level":"error","msg":"route(retry=0) dial tcp 127.0.0.1:59999: connect: connection refused","kind":"handler","sid":"bbb"}`
)

func TestDirectDecision(t *testing.T) {
	l := New(10)
	l.Feed(lineBypass)
	l.Feed(lineRouteD)
	es := l.Entries(0)
	if len(es) != 1 {
		t.Fatalf("entries = %d, want 1（同 sid 应归并）: %+v", len(es), es)
	}
	if es[0].Target != "192.168.1.10:8080" || es[0].Decision != Direct {
		t.Errorf("entry = %+v", es[0])
	}
}

func TestProxyDecisionWithNode(t *testing.T) {
	l := New(10)
	l.Feed(lineProxy)
	l.Feed(lineRouteP)
	es := l.Entries(0)
	if len(es) != 1 {
		t.Fatalf("entries = %+v", es)
	}
	e := es[0]
	if e.Target != "example.com:80" || e.Decision != Proxied {
		t.Errorf("entry = %+v", e)
	}
	if !strings.Contains(e.Node, "node-0@127.0.0.1:59999") {
		t.Errorf("node = %q", e.Node)
	}
}

// route 行本身就足以判定（即使没有 bypass 行）。
func TestRouteAloneDecides(t *testing.T) {
	l := New(10)
	l.Feed(`{"msg":"route(retry=0) 10.0.0.5:443","sid":"c1"}`)
	l.Feed(`{"msg":"route(retry=0) hop@1.1.1.1:80 > x.com:443","sid":"c2"}`)
	es := l.Entries(0)
	if len(es) != 2 {
		t.Fatalf("entries = %+v", es)
	}
	if es[0].Decision != Direct {
		t.Errorf("no prefix should be direct: %+v", es[0])
	}
	if es[1].Decision != Proxied || es[1].Node == "" {
		t.Errorf("node prefix should be proxied: %+v", es[1])
	}
}

func TestFailureRecorded(t *testing.T) {
	l := New(10)
	l.Feed(lineProxy)
	l.Feed(lineDialEr)
	es := l.Entries(0)
	if len(es) != 1 {
		t.Fatalf("entries = %+v", es)
	}
	if es[0].Err == "" {
		t.Error("failure reason should be recorded")
	}
	if !strings.Contains(es[0].Err, "连接被拒绝") {
		t.Errorf("err = %q", es[0].Err)
	}
}

// 多级链：目标取最后一段，链路作为节点。
func TestMultiHopChain(t *testing.T) {
	l := New(10)
	l.Feed(`{"msg":"route(retry=0) hop-0@1.1.1.1:80 > hop-1@2.2.2.2:80 > t.com:443","sid":"m1"}`)
	es := l.Entries(0)
	if len(es) != 1 {
		t.Fatalf("entries = %+v", es)
	}
	if es[0].Target != "t.com:443" {
		t.Errorf("target = %q", es[0].Target)
	}
	if !strings.Contains(es[0].Node, "hop-1@2.2.2.2:80") {
		t.Errorf("node = %q", es[0].Node)
	}
}

// 无关行被忽略，不产生记录。
func TestIgnoresUnrelated(t *testing.T) {
	l := New(10)
	l.Feed(`{"msg":"listening on [::]:8080/tcp","sid":""}`)
	l.Feed("plain text without json")
	l.Feed("")
	if n := l.Len(); n != 0 {
		t.Errorf("should ignore unrelated lines, got %d", n)
	}
}

// 关闭解析后不再产生记录。
func TestDisabled(t *testing.T) {
	l := New(10)
	l.SetEnabled(false)
	l.Feed(lineBypass)
	if n := l.Len(); n != 0 {
		t.Errorf("disabled should not record, got %d", n)
	}
	l.SetEnabled(true)
	l.Feed(lineBypass)
	if n := l.Len(); n != 1 {
		t.Errorf("enabled should record, got %d", n)
	}
}

// 环形缓冲满后保留最近的记录。
func TestRingBuffer(t *testing.T) {
	l := New(3)
	for i := 0; i < 5; i++ {
		l.Feed(`{"msg":"route(retry=0) t` + itoa(i) + `.com:443","sid":"s` + itoa(i) + `"}`)
	}
	es := l.Entries(0)
	if len(es) != 3 {
		t.Fatalf("entries = %d", len(es))
	}
	if es[2].Target != "t4.com:443" {
		t.Errorf("last = %q", es[2].Target)
	}
	if got := l.Entries(1); len(got) != 1 || got[0].Target != "t4.com:443" {
		t.Errorf("Entries(1) = %+v", got)
	}
}

func TestClear(t *testing.T) {
	l := New(10)
	l.Feed(lineBypass)
	l.Clear()
	if n := l.Len(); n != 0 {
		t.Errorf("after clear = %d", n)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
