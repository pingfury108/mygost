// Package profiles 管理配置档：每档对应一份 gost.yml。
package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Kind 是配置档类型。
type Kind string

const (
	KindProxy   Kind = "proxy"   // 本地代理（可带上游转发链）
	KindForward Kind = "forward" // 端口转发
	KindCustom  Kind = "custom"  // 自定义 YAML
)

// Profile 是一个配置档。
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind Kind   `json:"kind"`

	// KindProxy
	Listen   string `json:"listen"` // 如 :8080
	Proto    string `json:"proto"`  // http / socks5 / relay
	User     string `json:"user"`
	Password string `json:"password"`

	// 上游转发链（可选）
	UpEnabled bool   `json:"upEnabled"`
	UpProto   string `json:"upProto"` // http / socks5
	UpAddr    string `json:"upAddr"`  // host:port
	UpUser    string `json:"upUser"`
	UpPass    string `json:"upPass"`

	// KindForward
	FwdProto string `json:"fwdProto"` // tcp / udp
	Targets  string `json:"targets"`  // 每行一个 host:port

	// KindCustom：完整 gost.yml 内容
	YAML string `json:"yaml"`
}

// NewID 生成短随机 ID。
func NewID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// BuildYAML 由表单字段生成 gost v3 配置；KindCustom 原样返回。
// SplitConfig 描述生成 YAML 时要附加的分流配置。
type SplitConfig struct {
	// Enabled 是否启用分流。
	Enabled bool
	// PluginAddr 是 gost-geo-plugin 的 gRPC 地址。
	PluginAddr string
	// PrivateMatchers 是始终直连的私网段。
	PrivateMatchers []string
	// MetricsAddr 非空时给 gost 打开 Prometheus 端点（如 127.0.0.1:9100）。
	MetricsAddr string
}

// appendMetrics 在配置末尾追加 metrics 段（统计流量与连接）。
func appendMetrics(y []byte, addr string) []byte {
	s := strings.TrimRight(string(y), "\n")
	return []byte(s + "\nmetrics:\n  addr: \"" + addr + "\"\n")
}

// NoSplit 是未启用分流时的默认参数。
var NoSplit = SplitConfig{}

// BuildYAMLSplit 在 BuildYAML 的基础上附加分流配置。
//
// 附加三块：
//   - hop 引用 split-direct（黑名单命中 = 跳过上游 = 本机直连）
//   - split-private：私网段始终直连
//   - split-geo：白名单模式的 geo 插件（解析域名命中国内 IP → 允许直连）
func (p *Profile) BuildYAMLSplit(sc SplitConfig) ([]byte, error) {
	y, err := p.BuildYAML()
	if err != nil {
		return y, err
	}
	if sc.MetricsAddr != "" {
		y = appendMetrics(y, sc.MetricsAddr)
	}
	if !sc.Enabled || p.Kind != KindProxy {
		return y, nil
	}
	s := string(y)
	// bypass 插在 hop 名字行之后（属于该 hop 的键），而不是名字行之前
	hopRef := "      - name: hop-0\n"
	if idx := strings.Index(s, hopRef); idx >= 0 {
		pos := idx + len(hopRef)
		s = s[:pos] + "        bypass: split-direct\n" + s[pos:]
	}

	var b strings.Builder
	b.WriteString("\nbypasses:\n")

	b.WriteString("  - name: split-private\n    matchers:\n")
	for _, m := range sc.PrivateMatchers {
		b.WriteString("      - " + m + "\n")
	}

	b.WriteString("  - name: split-geo\n    whitelist: true\n    plugin:\n")
	b.WriteString("      type: grpc\n")
	b.WriteString("      addr: " + sc.PluginAddr + "\n")

	b.WriteString("  - name: split-direct\n    bypasses:\n")
	b.WriteString("      - split-private\n      - split-geo\n")

	s += b.String()
	return []byte(s), nil
}

func (p *Profile) BuildYAML() ([]byte, error) {
	if strings.TrimSpace(p.Listen) == "" {
		return nil, fmt.Errorf("监听地址不能为空")
	}
	var b strings.Builder
	switch p.Kind {
	case KindProxy:
		b.WriteString("services:\n")
		fmt.Fprintf(&b, "  - name: mygost\n")
		fmt.Fprintf(&b, "    addr: %q\n", p.Listen)
		b.WriteString("    handler:\n")
		fmt.Fprintf(&b, "      type: %s\n", orDefault(p.Proto, "http"))
		writeAuth(&b, 6, p.User, p.Password)
		if p.UpEnabled {
			b.WriteString("      chain: chain-0\n")
		}
		b.WriteString("    listener:\n")
		b.WriteString("      type: tcp\n")
		if p.UpEnabled {
			if strings.TrimSpace(p.UpAddr) == "" {
				return nil, fmt.Errorf("上游地址不能为空")
			}
			b.WriteString("chains:\n")
			b.WriteString("  - name: chain-0\n")
			b.WriteString("    hops:\n")
			b.WriteString("      - name: hop-0\n")
			b.WriteString("        nodes:\n")
			b.WriteString("          - name: node-0\n")
			fmt.Fprintf(&b, "            addr: %q\n", p.UpAddr)
			connectorType := orDefault(p.UpProto, "socks5")
			dialerType := DialerFor(connectorType)
			b.WriteString("            connector:\n")
			fmt.Fprintf(&b, "              type: %s\n", connectorType)
			if !authOnDialer(dialerType) {
				writeAuth(&b, 14, p.UpUser, p.UpPass)
			}
			b.WriteString("            dialer:\n")
			fmt.Fprintf(&b, "              type: %s\n", dialerType)
			if authOnDialer(dialerType) {
				writeAuth(&b, 14, p.UpUser, p.UpPass)
			}
		}
	case KindForward:
		targets := splitTargets(p.Targets)
		if len(targets) == 0 {
			return nil, fmt.Errorf("至少需要一个转发目标")
		}
		proto := orDefault(p.FwdProto, "tcp")
		b.WriteString("services:\n")
		b.WriteString("  - name: mygost\n")
		fmt.Fprintf(&b, "    addr: %q\n", p.Listen)
		b.WriteString("    handler:\n")
		fmt.Fprintf(&b, "      type: %s\n", proto)
		b.WriteString("    listener:\n")
		fmt.Fprintf(&b, "      type: %s\n", proto)
		b.WriteString("    forwarder:\n")
		b.WriteString("      nodes:\n")
		for i, t := range targets {
			fmt.Fprintf(&b, "        - name: target-%d\n", i)
			fmt.Fprintf(&b, "          addr: %q\n", t)
		}
	default:
		return nil, fmt.Errorf("未知类型 %q", p.Kind)
	}
	return []byte(b.String()), nil
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func writeAuth(b *strings.Builder, indent int, user, pass string) {
	if user == "" && pass == "" {
		return
	}
	pad := strings.Repeat(" ", indent)
	b.WriteString(pad + "auth:\n")
	fmt.Fprintf(b, "%s  username: %q\n", pad, user)
	fmt.Fprintf(b, "%s  password: %q\n", pad, pass)
}

// DialerTypes 是 gost 支持的连接方式（dialer），与 connector 是两个维度。
var DialerTypes = []string{
	"tcp", "udp", "tls", "mtls", "ws", "mws", "http2", "http3", "grpc", "quic",
	"kcp", "mtcp", "ssh", "sshd", "unix", "serial", "direct", "dtls", "ftcp",
	"icmp", "utls", "wg", "obfs", "pht", "masque",
}

// DialerFor 推导节点应使用的 dialer 类型。
//
// gost 命令行里 `sshd://host:port` 的 scheme 同时作为 connector 和 dialer，
// 因此只写 connector 而 dialer 留成 tcp 是错的：sshd connector 要求底层连接
// 已经是 SSH 会话（*ssh_util.ClientConn），否则报 "ssh: invalid connection"。
func DialerFor(connector string) string {
	c := strings.ToLower(strings.TrimSpace(connector))
	if c == "ssu" { // 与 gost 命令行一致：ssu 走 UDP
		return "udp"
	}
	for _, d := range DialerTypes {
		if d == c {
			return d
		}
	}
	return "tcp"
}

// authOnDialer 判断认证信息应写在 dialer 还是 connector 上。
// gost 对 ssh/sshd 把 SSH 登录凭据放在 dialer.auth，其余放在 connector.auth。
func authOnDialer(dialer string) bool {
	return dialer == "ssh" || dialer == "sshd"
}

// ConnectorTypes 是 gost 支持的转发链节点类型（connector），
// 从 gost v3 的注册表整理，按常用度排序。
var ConnectorTypes = []string{
	"socks5", "http", "sshd", "ssh", "ss", "relay", "forward", "sni",
	"socks4", "socks4a", "socks", "http2", "tunnel", "tcp", "unix",
	"direct", "masque", "router", "serial",
}

// KindLabel 返回类型显示名。
func KindLabel(k Kind) string {
	switch k {
	case KindForward:
		return "端口转发"
	case KindCustom:
		return "YAML"
	default:
		return "代理"
	}
}

func splitTargets(s string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' }) {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Store 管理目录下的配置档（每档一个 <id>.json）。
type Store struct {
	mu    sync.Mutex
	dir   string
	items []*Profile
}

// LoadStore 读取 dir 下全部配置档。
func LoadStore(dir string) (*Store, error) {
	s := &Store{dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var p Profile
		if json.Unmarshal(data, &p) == nil && p.ID != "" {
			s.items = append(s.items, &p)
		}
	}
	s.sortLocked()
	return s, nil
}

func (s *Store) sortLocked() {
	sort.Slice(s.items, func(i, j int) bool { return s.items[i].Name < s.items[j].Name })
}

// Items 返回配置档快照。
func (s *Store) Items() []*Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Profile, len(s.items))
	copy(out, s.items)
	return out
}

// Get 按 ID 查找，找不到返回 nil。
func (s *Store) Get(id string) *Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.items {
		if p.ID == id {
			cp := *p
			return &cp
		}
	}
	return nil
}

// Save 写入配置档并生成同名 .yml 供 gost 使用；返回 yml 路径。
func (s *Store) Save(p *Profile) (string, error) {
	return s.SaveSplit(p, NoSplit)
}

// SaveSplit 保存配置档并按分流配置生成 yml，返回 yml 路径。
func (s *Store) SaveSplit(p *Profile, sc SplitConfig) (string, error) {
	if strings.TrimSpace(p.Name) == "" {
		p.Name = "未命名"
	}
	yaml, err := p.BuildYAMLSplit(sc)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = NewID()
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(s.jsonPath(p.ID), data, 0o644); err != nil {
		return "", err
	}
	yml := s.yamlPath(p.ID)
	if err := os.WriteFile(yml, yaml, 0o644); err != nil {
		return "", err
	}
	replaced := false
	for i, it := range s.items {
		if it.ID == p.ID {
			cp := *p
			s.items[i] = &cp
			replaced = true
			break
		}
	}
	if !replaced {
		cp := *p
		s.items = append(s.items, &cp)
	}
	s.sortLocked()
	return yml, nil
}

// Summary 生成列表里的一行描述。
func (p *Profile) Summary() string {
	switch p.Kind {
	case KindForward:
		return p.Listen + " → " + firstLine(p.Targets)
	case KindCustom:
		return "自定义配置"
	default:
		s := p.Listen + " · " + p.Proto
		if p.UpEnabled {
			s += " · 经上游 " + p.UpAddr
		}
		return s
	}
}

func firstLine(s string) string {
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' }) {
		return strings.TrimSpace(line)
	}
	return ""
}

// Delete 删除配置档及其 yml。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.items {
		if it.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			os.Remove(s.jsonPath(id))
			os.Remove(s.yamlPath(id))
			return nil
		}
	}
	return fmt.Errorf("配置档不存在")
}

// YamlPath 返回某档的 yml 路径（不检查存在性）。
func (s *Store) YamlPath(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.yamlPath(id)
}

func (s *Store) jsonPath(id string) string { return filepath.Join(s.dir, id+".json") }
func (s *Store) yamlPath(id string) string { return filepath.Join(s.dir, id+".yml") }
