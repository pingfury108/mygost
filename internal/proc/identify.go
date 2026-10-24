package proc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Record 是 PID 文件的记录：让 mygost 在重启后认出自己启动的 gost。
type Record struct {
	PID     int       `json:"pid"`
	Profile string    `json:"profile"`
	Config  string    `json:"config"`
	Binary  string    `json:"binary"`
	Started time.Time `json:"started"`
}

// pidFile 返回 PID 文件路径。
func pidFile(dir string) string { return filepath.Join(dir, "gost.pid") }

// WritePIDFile 记录当前托管进程。
func WritePIDFile(dir string, rec Record) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pidFile(dir), data, 0o644)
}

// ReadPIDFile 读取并校验：进程必须存活，且命令行含记录里的 config。
// 校验失败（崩溃残留、pid 复用）时返回 false，由调用方走命令行扫描兜底。
func ReadPIDFile(dir string) (Record, bool) {
	var rec Record
	if dir == "" {
		return rec, false
	}
	data, err := os.ReadFile(pidFile(dir))
	if err != nil {
		return rec, false
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return rec, false
	}
	if rec.PID <= 0 || !ProcessAlive(rec.PID) {
		return rec, false
	}
	// PID 可能被系统复用：用命令行再确认一次是同一个配置
	if rec.Config != "" && !cmdlineContains(rec.PID, rec.Config) {
		return rec, false
	}
	return rec, true
}

// RemovePIDFile 删除 PID 文件。
func RemovePIDFile(dir string) {
	if dir == "" {
		return
	}
	_ = os.Remove(pidFile(dir))
}

// CommandlineContains 报告 pid 的命令行是否包含子串。
func CommandlineContains(pid int, sub string) bool { return cmdlineContains(pid, sub) }

// cmdlineContains 报告 pid 的命令行是否包含子串。
func cmdlineContains(pid int, sub string) bool {
	if pid <= 0 || sub == "" {
		return false
	}
	out, err := processCmdline(pid)
	if err != nil {
		return false
	}
	return strings.Contains(out, sub)
}

// parsePids 从逐行输出里解析 pid（跳过非数字行）。
func parsePids(s string, _ int) []int {
	var out []int
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "p") { // lsof -Fp 前缀
			line = strings.TrimPrefix(line, "p")
		}
		if p, err := strconv.Atoi(line); err == nil && p > 0 {
			out = append(out, p)
		}
	}
	return out
}

// ErrNoProcess 表示没有可接管的进程。
var ErrNoProcess = fmt.Errorf("没有可接管的 gost 进程")
