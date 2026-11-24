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
	// 文件必须排在 stderr 前面：Windows GUI 子系统没有控制台，
	// stderr 写入会失败，而 io.MultiWriter 遇到第一个错误就返回。
	// 先写文件，再尝试 stderr，才能保证日志真正落盘。
	var stderr io.Writer
	if os.Stderr != nil {
		stderr = os.Stderr
	}
	log.SetOutput(logOutput(f, stderr))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("=== mygost %s 启动，数据目录 %s ===", appVersion, dir)
	return path, func() { _ = f.Close() }
}

// logOutput 组合文件与控制台输出。
//
// stderr 为 nil（Windows GUI 下可能没有标准错误句柄）时只写文件；
// 非 nil 时忽略它的写入错误，避免控制台问题影响文件日志。
func logOutput(f io.Writer, stderr io.Writer) io.Writer {
	if stderr == nil {
		return f
	}
	return io.MultiWriter(f, ignoreWriteError{w: stderr})
}

// ignoreWriteError 吞掉控制台写入错误，并报告成功字节数，
// 让 MultiWriter 继续处理后续 writer。
type ignoreWriteError struct{ w io.Writer }

func (w ignoreWriteError) Write(p []byte) (int, error) {
	_, _ = w.w.Write(p)
	return len(p), nil
}
