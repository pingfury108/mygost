package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygost/internal/reqlog"
)

// 日期选择行应列出有记录的历史日期（实时 + 日期…）。
func TestRequestDayRow(t *testing.T) {
	a := newTestApp(t)
	a.page = "logs"
	a.logView = 1
	a.reqDay = ""

	yesterday := time.Now().AddDate(0, 0, -1)
	if st := a.reqs.Store(); st != nil {
		st.Append(reqlog.Entry{At: yesterday, Target: "hist.com:443",
			SID: "h1", Decision: reqlog.Proxied})
		st.Flush()
	}

	// 传入 sel=历史日期，模拟用户选了昨天，列表应显示那天的记录
	a.reqDay = yesterday.Format("2006-01-02")
	tt := ui.NewTester(a.view, 430, 700)
	if !tt.HasText("hist.com:443") {
		t.Errorf("应显示选中日期的记录, texts %q", tt.Texts())
	}
	texts := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, yesterday.Format("2006-01-02")) {
		t.Errorf("下拉应包含历史日期 %s", yesterday.Format("2006-01-02"))
	}
}

// 切到历史日期后，列表应显示那天的记录而不是实时记录。
func TestRequestHistoryLoad(t *testing.T) {
	a := newTestApp(t)
	a.page = "logs"
	a.logView = 1

	yesterday := time.Now().AddDate(0, 0, -1)
	day := yesterday.Format("2006-01-02")
	if st := a.reqs.Store(); st != nil {
		st.Append(reqlog.Entry{At: yesterday, Target: "old-record.com:443",
			SID: "old1", Decision: reqlog.Direct})
		st.Flush()
	}

	// 实时视图注入一条今天的记录，确保列表内容确实来自历史
	a.reqs.Feed(`{"msg":"bypass: today.com:443","sid":"t1"}`)

	a.reqDay = day
	tt := ui.NewTester(a.view, 430, 700)
	if !tt.HasText("old-record.com:443") {
		t.Errorf("应显示历史记录, texts %q", tt.Texts())
	}
	if tt.HasText("today.com:443") {
		t.Errorf("历史视图不应混入实时记录, texts %q", tt.Texts())
	}
}
