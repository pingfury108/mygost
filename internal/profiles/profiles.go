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
// DefaultPrivateMatchers 是始终直连的本地/私有网段。
//
// 这些地址不该经过隧道：走了隧道就等于让远端服务器去访问它自己的局域网，
// 轻则连不上，重则误连到远端内网的同 IP 设备。
var DefaultPrivateMatchers = []string{
	// IPv4 私有
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	// 本机与链路本地
	"127.0.0.0/8", "169.254.0.0/16", "0.0.0.0/8",
	// 运营商级 NAT（Tailscale、部分 CGNAT 网络）
	"100.64.0.0/10",
	// 组播与广播（mDNS/DLNA 设备发现、受限广播）
	"224.0.0.0/4", "255.255.255.255/32",
	// IPv6
	"::1/128", "::/128", "fc00::/7", "fe80::/10", "ff00::/8",
}

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
	// DebugLog 为 true 时把 gost 日志级别设为 debug（请求决策只在 debug 输出）。
	DebugLog bool
}

// appendLogLevel 追加日志级别设置。
func appendLogLevel(y []byte, level string) []byte {
	s := strings.TrimRight(string(y), "\n")
	return []byte(s + "\nlog:\n  level: " + level + "\n")
}

// appendMetrics 在配置末尾追加 metrics 段（统计流量与连接）。
func appendMetrics(y []byte, addr string) []byte {
	s := strings.TrimRight(string(y), "\n")
	return []byte(s + "\nmetrics:\n  addr: \"" + addr + "\"\n")
}

// NoSplit 是未启用分流时的默认参数。
var NoSplit = SplitConfig{}

// BuildYAMLSplit 在 BuildYAML 的基础上附加 bypass 与 metrics。
//
// bypass 分两层：
//   - split-private：私网/本机/组播段直连。**只要有上游就始终生效**，
//     否则局域网流量会被送到远端服务器去连它自己的内网。
//   - split-geo：国内 IP 直连（白名单模式，由 geo 插件判断）。仅分流开启时加入。
//
// 两者由 split-direct 组合，挂在 hop 上：命中即跳过上游，由本机直连。
func (p *Profile) BuildYAMLSplit(sc SplitConfig) ([]byte, error) {
	y, err := p.BuildYAML()
	if err != nil {
		return y, err
	}
	if sc.MetricsAddr != "" {
		y = appendMetrics(y, sc.MetricsAddr)
	}
	if sc.DebugLog {
		y = appendLogLevel(y, "debug")
	}
	// 没有上游链就没有 hop，bypass 无处可挂（本机直连本就是默认行为）
	if p.Kind != KindProxy || !p.UpEnabled {
		return y, nil
	}

	useGeo := sc.Enabled && sc.PluginAddr != ""

	s := string(y)
	// 注意：gost 3.3.1 的配置解析器不支持 bypasses 里再嵌套引用其它 bypass
	// （ParseBypass 只看 matchers/plugin/file），因此不能用"group"包一层。
	// 正确写法是在 hop 上直接列多个 bypass —— HopConfig.Bypasses 会被
	// 合并成 BypassGroup：白名单（geo）用 AND、黑名单（私网）用 OR。
	hopRef := "      - name: hop-0\n"
	if idx := strings.Index(s, hopRef); idx >= 0 {
		pos := idx + len(hopRef)
		list := "        bypasses:\n          - split-private\n"
		if useGeo {
			list += "          - split-geo\n"
		}
		s = s[:pos] + list + s[pos:]
	}

	var b strings.Builder
	b.WriteString("\nbypasses:\n")

	matchers := sc.PrivateMatchers
	if len(matchers) == 0 {
		matchers = DefaultPrivateMatchers // 调用方未指定时的安全默认
	}
	b.WriteString("  - name: split-private\n    matchers:\n")
	for _, m := range matchers {
		b.WriteString("      - " + m + "\n")
	}

	if useGeo {
		b.WriteString("  - name: split-geo\n    whitelist: true\n    plugin:\n")
		b.WriteString("      type: grpc\n")
		b.WriteString("      addr: " + sc.PluginAddr + "\n")
	}

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
