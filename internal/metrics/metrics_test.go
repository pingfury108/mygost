package metrics

import (
	"strings"
	"testing"
	"time"
)

const sample = `# HELP gost_service_transfer_input_bytes_total Total service input data transfer size in bytes
# TYPE gost_service_transfer_input_bytes_total counter
gost_service_transfer_input_bytes_total{client="127.0.0.1:1",host="x",service="mygost"} 1000
gost_service_transfer_input_bytes_total{client="127.0.0.1:2",host="y",service="mygost"} 500
gost_service_transfer_output_bytes_total{client="127.0.0.1:1",host="x",service="mygost"} 4000
gost_service_requests_total{client="127.0.0.1:1"} 7
gost_service_handler_errors_total{client="127.0.0.1:1"} 2
gost_service_requests_inflight{client="127.0.0.1:1"} 3
other_metric 999
`

func TestParseAggregatesByMetricName(t *testing.T) {
	s, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if s.Input != 1500 {
		t.Errorf("input = %v, want 1500（按标签求和）", s.Input)
	}
	if s.Output != 4000 {
		t.Errorf("output = %v", s.Output)
	}
	if s.Reqs != 7 || s.Errors != 2 {
		t.Errorf("reqs/errors = %v/%v", s.Reqs, s.Errors)
	}
}

func TestSplitSample(t *testing.T) {
	cases := []struct {
		line string
		name string
		val  float64
		ok   bool
	}{
		{`a{x="1"} 12.5`, "a", 12.5, true},
		{"b 3", "b", 3, true},
		{"# comment", "", 0, false},
		{"c", "", 0, false},
		{"d 1e+06", "d", 1e6, true},
	}
	for _, c := range cases {
		name, val, ok := splitSample(c.line)
		if ok != c.ok || (ok && (name != c.name || val != c.val)) {
			t.Errorf("splitSample(%q) = %q,%v,%v", c.line, name, val, ok)
		}
	}
}

func snap(in, out float64, at time.Time) Snapshot {
	return Snapshot{Input: in, Output: out, At: at}
}

// 速率 = 字节差 / 时间差；首帧无速率。
func TestSpeedRates(t *testing.T) {
	sp := NewSpeed(1) // 不平滑，方便断言
	base := time.Now()
	r := sp.Observe(snap(0, 0, base))
	if r.Up != 0 || r.Down != 0 {
		t.Fatalf("first sample should have no rate: %+v", r)
	}
	r = sp.Observe(snap(1000, 2000, base.Add(time.Second)))
	if r.Up != 1000 || r.Down != 2000 {
		t.Errorf("rates = %+v, want 1000/2000", r)
	}
	if r.TotalUp != 1000 || r.TotalDown != 2000 {
		t.Errorf("totals = %+v", r)
	}
}

// 计数器归零（gost 重启）：不产生负速度，并重取基准。
func TestSpeedCounterReset(t *testing.T) {
	sp := NewSpeed(1)
	now := time.Now()
	sp.Observe(snap(10000, 20000, now))
	r := sp.Observe(snap(10, 20, now.Add(time.Second)))
	if r.Up < 0 || r.Down < 0 {
		t.Fatalf("negative rate after reset: %+v", r)
	}
	// 重置后累计值应从新基准算起
	r = sp.Observe(snap(1010, 2020, now.Add(2*time.Second)))
	if r.TotalUp != 1000 {
		t.Errorf("total after reset = %v, want 1000", r.TotalUp)
	}
}

// 滑动平均抹平抖动。
func TestSpeedSmoothing(t *testing.T) {
	sp := NewSpeed(2)
	base := time.Now()
	sp.Observe(snap(0, 0, base))
	sp.Observe(snap(1000, 0, base.Add(time.Second)))        // 1000/s
	r := sp.Observe(snap(1000, 0, base.Add(2*time.Second))) // 0/s，均值应为 500
	if r.Up != 500 {
		t.Errorf("smoothed = %v, want 500", r.Up)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[float64]string{
		512:           "512 B",
		1536:          "1.5 KB",
		1024 * 1024:   "1 MB",
		5_368_709_120: "5 GB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%v) = %q, want %q", in, got, want)
		}
	}
}

// 端点不可用时不 panic，返回错误。
func TestScrapeNoEndpoint(t *testing.T) {
	c := NewClient("")
	if _, err := c.Scrape(); err == nil {
		t.Error("should error without endpoint")
	}
}
