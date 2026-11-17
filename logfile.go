package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// logMaxBytes 是日志文件的上限：超过就截断重来（不做历史轮转，
// 这里只是排障用，保留最近一段即可）。
const logMaxBytes = 2 << 20 // 2 MB

// setupLogging 把标准日志同时写入文件。
//
// Windows 上应用以 GUI 子系统发布（-H=windowsgui），没有控制台，
// os.Stderr 是无效句柄——log 的输出会被直接丢弃。结果就是：双击启动
// 出问题时什么都看不到，只能从 cmd 启动才"碰巧"能看到错误。
//
// 因此无条件落一份文件日志，路径在数据目录下，设置页可打开。
func setupLogging(dir string) (logPath string, closeFn func()) {
	path := filepath.Join(dir, "logs", "mygost.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", func() {}
	}
	// 超限时先清空，避免无限增长
	if st, err := os.Stat(path); err == nil && st.Size() > logMaxBytes {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("=== mygost %s 启动，数据目录 %s ===", appVersion, dir)
	return path, func() { _ = f.Close() }
}
