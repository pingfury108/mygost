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

	// external 表示当前地址上的插件不是本实例拉起的（上次遗留或别的实例），
	// 只是复用；Stop 时不应该去杀它。
	external bool
	lastErr  string

	// counts 缓存列表行数：域名表 11 万行，设置页每帧都会问一次，
	// 每次重读文件会让滚动明显卡顿。按 mtime+size 失效。
	counts map[string]lineCount
}

// lineCount 是一条列表文件的行数缓存。
type lineCount struct {
	mod  time.Time
	size int64
	n    int
}

// NewManager 创建管理器；dir 存放插件二进制与列表文件。
func NewManager(dir string, port int) *Manager {
	if port <= 0 {
		port = DefaultListenPort
	}
	return &Manager{
		dir: dir, port: port, policy: PolicyProxy,
		counts: make(map[string]lineCount),
	}
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

// Running 报告自有进程是否还在（不代表服务可用，看 Alive）。
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.Process != nil
}

// Alive 报告插件服务是否可用：自有进程活着，或地址上已有可用实例。
//
// 判定以“服务能不能用”为准，不看进程归属：app 重启后自有 cmd 是 nil，
// 但上次遗留的 splitd 仍在监听，此时应该直接复用而不是报未运行。
func (m *Manager) Alive() bool { return m.Running() || portReachable(m.Addr()) }

// Status 是插件状态快照（供界面渲染，不做事）。
type Status struct {
	Alive    bool   // 服务可用
	External bool   // 复用非本实例启动的插件
	Addr     string // 实际监听地址
	Err      string // 最近一次失败原因
}

// Status 返回当前状态快照。
func (m *Manager) Status() Status {
	m.mu.Lock()
	ext, errStr := m.external, m.lastErr
	m.mu.Unlock()
	return Status{Alive: m.Alive(), External: ext, Addr: m.Addr(), Err: errStr}
}

func (m *Manager) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		m.lastErr = ""
	} else {
		m.lastErr = err.Error()
	}
}

// portReachable 报告地址上是否有人在监听。
func portReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ensurePort 确保当前端口可用：被其他程序占用时换一个空闲端口。
//
// 8100 是常用端口，可能被用户的其它服务占着；此时换端口而不是报错。
func (m *Manager) ensurePort() error {
	m.mu.Lock()
	port := m.port
	m.mu.Unlock()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		ln.Close()
		return nil
	}
	// 端口被占：从 8100 往后找一个空闲的
	free, perr := pickFreePort(8410, 60)
	if perr != nil {
		return fmt.Errorf("端口 %d 被占用，且找不到空闲端口: %w", port, perr)
	}
	m.mu.Lock()
	m.port = free
	m.mu.Unlock()
	return nil
}

// pickFreePort 从 start 起找第一个可监听的端口。
func pickFreePort(start, tries int) (int, error) {
	for p := start; p < start+tries; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		ln.Close()
		return p, nil
	}
	return 0, fmt.Errorf("区间 %d-%d 无可用端口", start, start+tries)
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
	if err := m.start(ctx); err != nil {
		m.setErr(err)
		return err
	}
	m.setErr(nil)
	return nil
}

func (m *Manager) start(ctx context.Context) error {
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

	// ① 地址上已有插件（上次遗留/其他实例）→ 直接复用，避开端口碰撞
	if portReachable(m.Addr()) {
		m.mu.Lock()
		m.external = true
		m.mu.Unlock()
		return nil
	}

	// ② 默认端口被其他程序占用 → 换一个空闲端口
	if err := m.ensurePort(); err != nil {
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
// Stop 停止插件。
//
// 只杀本实例拉起的进程：复用的外部实例（上次遗留/其他实例）不归我们管，
// 杀了会误伤。要连外部实例一起停的语义由 ForceStop 提供。
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
	m.external = false
}

// ForceStop 停掉本实例进程，并在地址上仍有人监听时返回错误提示。
//
// 用于“分流关闭”场景：插件不该继续占着端口，但外部实例需要显式清理。
func (m *Manager) ForceStop() error {
	m.Stop()
	if portReachable(m.Addr()) {
		return fmt.Errorf("端口 %s 上仍有外部插件实例", m.Addr())
	}
	return nil
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
//
// 结果按 mtime+size 缓存：列表文件是 11 万行级别，设置页每帧渲染都会问，
// 不缓存会让滚动卡成幻灯片。文件被重写后 mtime 变化，自动失效。
func (m *Manager) CountLines(path string) int {
	if path == "" {
		return -1
	}
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	m.mu.Lock()
	if c, ok := m.counts[path]; ok && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		m.mu.Unlock()
		return c.n
	}
	m.mu.Unlock()

	n := countFileLines(path)
	m.mu.Lock()
	if m.counts == nil {
		m.counts = make(map[string]lineCount)
	}
	m.counts[path] = lineCount{mod: st.ModTime(), size: st.Size(), n: n}
	m.mu.Unlock()
	return n
}

// countFileLines 真读文件统计有效行（非空、非注释）。
func countFileLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
