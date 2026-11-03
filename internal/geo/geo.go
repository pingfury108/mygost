// Package geo 管理分流插件（splitd）进程与两份规则列表的下载更新。
//
// 插件按「CN 域名 → 直连；CN IP → 直连；其余 → 代理」判定，
// 直连由本机解析（就近 CDN），代理由上游解析（避开本地 DNS 污染）。
package geo

import (
	"bufio"
	"context"
	"encoding/json"
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

	"mygost/internal/profiles"
	"mygost/internal/splitbin"
)

// DefaultIPListURL 是国内 IP 列表（Loyalsoldier 每日构建，纯 CIDR 文本）。
const DefaultIPListURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt"

// DefaultDomainListURL 是国内域名列表（MetaCubeX sing-box geosite cn）。
const DefaultDomainListURL = "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/cn.json"

// DefaultListenPort 是插件 gRPC 监听端口。
const DefaultListenPort = 8100

// PrivateMatchers 复用 profiles 的默认私网段，避免两处维护。
var PrivateMatchers = profiles.DefaultPrivateMatchers

// Policy 是未收录域名的处理策略（与 splitd 保持一致）。
type Policy string

const (
	// PolicyProxy 未收录域名走代理（默认）。
	PolicyProxy Policy = "proxy"
	// PolicyResolve 未收录域名本地解析后查 IP 表。
	PolicyResolve Policy = "resolve"
)

// Manager 托管分流插件进程。
type Manager struct {
	mu     sync.Mutex
	dir    string
	port   int
	policy Policy
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

// NewManager 创建管理器；dir 存放插件二进制与列表文件。
func NewManager(dir string, port int) *Manager {
	if port <= 0 {
		port = DefaultListenPort
	}
	return &Manager{dir: dir, port: port, policy: PolicyProxy}
}

// SetPolicy 设置未收录域名的策略（下次启动生效）。
func (m *Manager) SetPolicy(p Policy) {
	m.mu.Lock()
	if p == "" {
		p = PolicyProxy
	}
	m.policy = p
	m.mu.Unlock()
}

// PluginPath 返回插件二进制路径。
func (m *Manager) PluginPath() string {
	name := "splitd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(m.dir, name)
}

// IPListPath 返回 IP 列表文件路径。
func (m *Manager) IPListPath() string { return filepath.Join(m.dir, "geoip-cn.txt") }

// DomainListPath 返回域名列表文件路径。
func (m *Manager) DomainListPath() string { return filepath.Join(m.dir, "geosite-cn.txt") }

// Addr 返回插件 gRPC 地址。
func (m *Manager) Addr() string { return fmt.Sprintf("127.0.0.1:%d", m.port) }

// Installed 报告插件是否就位（数据目录或嵌入副本）。
func (m *Manager) Installed() bool {
	if st, err := os.Stat(m.PluginPath()); err == nil && !st.IsDir() {
		return true
	}
	return len(splitbin.Plugin()) > 0
}

// Ready 报告两份列表是否都已就位。
func (m *Manager) Ready() bool { return fileReady(m.IPListPath()) && fileReady(m.DomainListPath()) }

func fileReady(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Size() > 0
}

// Running 报告插件进程是否在运行。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

// ensureInstalled 把嵌入的插件写到数据目录（幂等）。
func (m *Manager) ensureInstalled() error {
	if st, err := os.Stat(m.PluginPath()); err == nil && !st.IsDir() {
		return nil
	}
	bin := splitbin.Plugin()
	if len(bin) == 0 {
		return fmt.Errorf("分流插件未嵌入且未安装")
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

// Start 启动插件（幂等）。两份列表都必须已存在。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.mu.Unlock()
		return nil
	}
	policy := m.policy
	m.mu.Unlock()

	if !m.Ready() {
		return fmt.Errorf("规则列表未下载")
	}
	if err := m.ensureInstalled(); err != nil {
		return err
	}

	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, m.PluginPath(),
		"--listen", m.Addr(),
		"--ip-list", m.IPListPath(),
		"--domain-list", m.DomainListPath(),
		"--policy", string(policy),
	)
	// 插件日志保留到管道，便于诊断（每 10 分钟一行重载信息）
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return err
	}
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		cancel()
		pw.Close()
		pr.Close()
		return err
	}
	pw.Close()

	m.mu.Lock()
	m.cmd, m.cancel = cmd, cancel
	m.mu.Unlock()

	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			_ = sc.Text() // 插件日志当前仅用于避免管道阻塞
		}
		pr.Close()
	}()
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

// WaitReady 轮询等待插件端口可连。
func (m *Manager) WaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", m.Addr(), 500*time.Millisecond)
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
	return fmt.Errorf("分流插件启动超时")
}

// UpdateIPList 下载国内 IP 列表，返回条目数。
func (m *Manager) UpdateIPList(ctx context.Context, url string) (int, error) {
	if strings.TrimSpace(url) == "" {
		url = DefaultIPListURL
	}
	body, err := openURL(ctx, url, 8*time.Minute)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	return m.writeList(body, m.IPListPath(), func(line string) string {
		return line // CIDR 原样
	})
}

// UpdateDomainList 下载 geosite 的 cn.json 并转成后缀列表，返回条目数。
func (m *Manager) UpdateDomainList(ctx context.Context, url string) (int, error) {
	if strings.TrimSpace(url) == "" {
		url = DefaultDomainListURL
	}
	body, err := openURL(ctx, url, 10*time.Minute)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	domains, err := ParseGeositeJSON(body)
	if err != nil {
		return 0, err
	}
	if len(domains) == 0 {
		return 0, fmt.Errorf("域名列表为空")
	}
	return m.writeLines(domains, m.DomainListPath())
}

// openURL 打开 http(s) 或 file:// 资源。
func openURL(ctx context.Context, url string, timeout time.Duration) (io.ReadCloser, error) {
	if strings.HasPrefix(url, "file://") {
		return os.Open(strings.TrimPrefix(url, "file://"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// writeList 逐行清洗后写入 dst（原子替换）。
func (m *Manager) writeList(body io.Reader, dst string, clean func(string) string) (int, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriter(f)
	n := 0
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(clean(sc.Text()))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, err := w.WriteString(line + "\n"); err != nil {
			f.Close()
			os.Remove(tmp)
			return 0, err
		}
		n++
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
	if n == 0 {
		os.Remove(tmp)
		return 0, fmt.Errorf("列表为空")
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, nil
}

// writeLines 写入已准备好的条目。
func (m *Manager) writeLines(lines []string, dst string) (int, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, err := w.WriteString(l + "\n"); err != nil {
			f.Close()
			os.Remove(tmp)
			return 0, err
		}
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
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return len(lines), nil
}

// ParseGeositeJSON 解析 sing-box 的 geosite JSON，提取 domain_suffix。
func ParseGeositeJSON(r io.Reader) ([]string, error) {
	var payload struct {
		Rules []struct {
			DomainSuffix []string `json:"domain_suffix"`
		} `json:"rules"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 geosite JSON: %w", err)
	}
	seen := make(map[string]struct{}, 1024)
	out := make([]string, 0, 1024)
	for _, rule := range payload.Rules {
		for _, s := range rule.DomainSuffix {
			s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, ".")))
			if s == "" {
				continue
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out, nil
}

// CountLines 返回文件行数（-1 表示无文件）。
func (m *Manager) CountLines(path string) int {
	if path == "" {
		return -1
	}
	f, err := os.Open(path)
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

// CountIP 返回 IP 列表条目数。
func (m *Manager) CountIP() int { return m.CountLines(m.IPListPath()) }

// CountDomains 返回域名列表条目数。
func (m *Manager) CountDomains() int { return m.CountLines(m.DomainListPath()) }

// UpdateAt 返回文件的最后修改时间。
func (m *Manager) UpdateAt(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	st, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// InstallFromPath 从本地路径安装插件（手动覆盖升级用）。
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

// PluginBytes 返回嵌入的插件二进制（测试与诊断用）。
func PluginBytes() []byte { return splitbin.Plugin() }

// ErrNoProcess 表示没有可接管的进程。
var ErrNoProcess = fmt.Errorf("没有可接管的进程")
