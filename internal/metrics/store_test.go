package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAddAndPersist(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	date := today(time.Now())
	s.Add(date, 1000, 2000, 60)
	s.AddSession(date)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	// 重新加载应保留数据
	s2, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	d := s2.Today()
	if d.Up != 1000 || d.Down != 2000 || d.Sessions != 1 || d.Seconds != 60 {
		t.Errorf("reloaded day = %+v", d)
	}
}

// 超过 Retention 天的记录必须被丢弃。
func TestStorePrunesOldData(t *testing.T) {
	dir := t.TempDir()
	// 手写一份含旧数据的 stats.json
	old := time.Now().AddDate(0, 0, -(Retention + 5)).Format("2006-01-02")
	recent := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	body := `{"days":[{"date":"` + old + `","up":1,"down":2},{"date":"` + recent + `","up":3,"down":4}]}`
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Fatalf("old data not pruned, days = %d", s.Len())
	}
	if got := s.Recent(10); len(got) != 1 || got[0].Date != recent {
		t.Errorf("recent = %+v", got)
	}
}

// 保留窗口边界：正好 Retention 天内的数据要留下。
func TestStoreKeepsWithinRetention(t *testing.T) {
	dir := t.TempDir()
	keep := time.Now().AddDate(0, 0, -(Retention - 1)).Format("2006-01-02")
	body := `{"days":[{"date":"` + keep + `","up":5,"down":6}]}`
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 {
		t.Errorf("data within retention should be kept, got %d days", s.Len())
	}
}

func TestStoreTotals(t *testing.T) {
	s, _ := LoadStore(t.TempDir())
	now := time.Now()
	s.Add(now.Format("2006-01-02"), 100, 200, 10)
	s.Add(now.AddDate(0, 0, -1).Format("2006-01-02"), 300, 400, 20)
	tot := s.Total()
	if tot.Up != 400 || tot.Down != 600 || tot.Seconds != 30 {
		t.Errorf("total = %+v", tot)
	}
	if len(s.Recent(10)) != 2 {
		t.Errorf("recent = %+v", s.Recent(10))
	}
	// Recent(1) 只保留最后一天
	if got := s.Recent(1); len(got) != 1 || got[0].Up != 100 {
		t.Errorf("Recent(1) = %+v", got)
	}
}

// 损坏的 stats.json 不应阻塞启动。
func TestStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stats.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadStore(dir)
	if err != nil {
		t.Fatalf("corrupt file should not fail: %v", err)
	}
	if s.Len() != 0 {
		t.Errorf("should start empty, got %d", s.Len())
	}
}

// 无变化时 Save 不写盘。
func TestStoreSaveNoChange(t *testing.T) {
	dir := t.TempDir()
	s, _ := LoadStore(dir)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stats.json")); err == nil {
		t.Error("should not write when nothing changed")
	}
}
