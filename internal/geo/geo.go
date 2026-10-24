// Package geo 管理 gost-geo-plugin 子进程与 GeoIP 规则文件的下载更新。
//
// 插件以 gRPC bypass 插件形式与 gost 协作：
// gost 把目标地址发给插件，插件解析域名后查 CIDR trie 命中国内 IP 则直连。
package geo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mygost/internal/geobin"
)

// PluginURL 是 gost-geo-plugin 的 go install 包路径（来源 go-gost 官方）。
const PluginModule = "github.com/go-gost/gost-geo-plugin@latest"

// DefaultIPListURL 是默认的国内 IP 列表（Loyalsoldier 每日构建，纯 CIDR 文本）。
const DefaultIPListURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt"

// DefaultListenPort 是插件 gRPC 监听端口。
const DefaultListenPort = 8100

// PrivateMatchers 是始终直连的私网段（写入 gost 配置的静态 bypass）。
var PrivateMatchers = []string{
	"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"169.254.0.0/16", "::1/128", "fc00::/7", "fe80::/10",
}

// Manager 托管 geo 插件进程。
type Manager struct {
	mu     sync.Mutex
	dir    string // 插件与规则文件所在目录
	port   int
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// NewManager 创建管理器，dir 存放插件二进制与 geoip-cn.txt。
func NewManager(dir string, port int) *Manager {
	if port <= 0 {
		port = DefaultListenPort
	}
	return &Manager{dir: dir, port: port}
}

// PluginPath 返回插件二进制路径。
func (m *Manager) PluginPath() string {
	name := "gost-geo-plugin"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.dir, name)
}

// IPListPath 返回国内 IP 列表文件路径。
func (m *Manager) IPListPath() string {
	if m == nil {
		return ""
	}
	return filepath.Join(m.dir, "geoip-cn.txt")
}

// Addr 返回插件 gRPC 地址。
func (m *Manager) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", m.port)
}

// Installed 报告插件是否已就位（数据目录或嵌入副本）。
func (m *Manager) Installed() bool {
	if st, err := os.Stat(m.PluginPath()); err == nil && !st.IsDir() {
		return true
	}
	return len(embedded()) > 0
}

// embedded 返回构建时嵌入的插件二进制（默认构建恒有）。
func embedded() []byte { return geobin.Plugin() }

// ensureInstalled 把嵌入的插件写到数据目录（幂等，按大小判断是否需重写）。
func (m *Manager) ensureInstalled() error {
	if st, err := os.Stat(m.PluginPath()); err == nil && !st.IsDir() {
		return nil // 已存在
	}
	bin := embedded()
	if len(bin) == 0 {
		return fmt.Errorf("插件未嵌入且未安装")
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	tmp := m.PluginPath() + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	return os.Rename(tmp, m.PluginPath())
}

// ListReady 报告规则文件是否已就位。
func (m *Manager) ListReady() bool {
	st, err := os.Stat(m.IPListPath())
	return err == nil && !st.IsDir() && st.Size() > 0
}

// Running 报告插件进程是否在运行。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

// Start 启动插件（幂等）：先确保规则文件存在，再拉起进程。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd != nil && m.cmd.Process != nil {
		return nil
	}
	if !m.ListReady() {
		return fmt.Errorf("IP 列表未下载")
	}
	if err := m.ensureInstalled(); err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, m.PluginPath(),
		"--port", fmt.Sprint(m.port),
		"--listen-addr", "127.0.0.1",
		"--list-url", "file://"+m.IPListPath(),
	)
	// 插件日志丢弃（其输出冗长且每连接一行），状态以端口探活为准
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}
	m.cmd, m.cancel = cmd, cancel

	// 进程退出时清理句柄
	go func() { _ = cmd.Wait() }()
	return nil
}

// Stop 停止插件进程。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}
	m.cmd, m.cancel = nil, nil
}

// WaitReady 轮询等待插件端口可连（最多 timeout），启动即完成 IP 列表加载。
func (m *Manager) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", m.port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("geo 插件启动超时")
}

// UpdateIPList 下载国内 IP 列表，返回条目数。写入临时文件后原子替换。
// url 支持 file://（读取本地文件）与 http(s)://。
func (m *Manager) UpdateIPList(ctx context.Context, url string) (int, error) {
	if strings.TrimSpace(url) == "" {
		url = DefaultIPListURL
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return 0, err
	}
	var body io.ReadCloser
	if strings.HasPrefix(url, "file://") {
		f, err := os.Open(strings.TrimPrefix(url, "file://"))
		if err != nil {
			return 0, err
		}
		body = f
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return 0, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
		}
		body = resp.Body
	}
	defer body.Close()

	tmp := m.IPListPath() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	count := 0
	sc := bufio.NewScanner(body)
	w := bufio.NewWriter(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := w.WriteString(line + "\n"); err != nil {
			f.Close()
			os.Remove(tmp)
			return 0, err
		}
		count++
	}
	if err := sc.Err(); err != nil {
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	if count == 0 {
		os.Remove(tmp)
		return 0, fmt.Errorf("列表为空")
	}
	if err := os.Rename(tmp, m.IPListPath()); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return count, nil
}

// CountLines 返回规则文件行数（-1 表示无文件）。
func (m *Manager) CountLines() int {
	if m == nil || m.IPListPath() == "" {
		return -1
	}
	f, err := os.Open(m.IPListPath())
	if err != nil {
		return -1
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" && !strings.HasPrefix(s, "#") {
			n++
		}
	}
	return n
}

// UpdateAt 返回规则文件最后修改时间。
func (m *Manager) UpdateAt() time.Time {
	if m == nil || m.IPListPath() == "" {
		return time.Time{}
	}
	st, err := os.Stat(m.IPListPath())
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// InstallFromPath 从本地路径安装插件二进制，覆盖嵌入副本（手动升级用）。
// 正常情况下无需调用：构建时嵌入的插件会自动解压安装。
func (m *Manager) InstallFromPath(src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	tmp := m.PluginPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmp, 0o755)
	}
	return os.Rename(tmp, m.PluginPath())
}
