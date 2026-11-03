// Package splitd 是 mygost 的分流判定引擎与 gRPC 插件服务。
//
// 它替代 gost-geo-plugin：后者只按"域名解析出的 IP 是否属于 CN"判定，
// 导致 iCloud/Apple 这类解析到国内 CDN 节点的国外域名被误判为直连。
//
// 本引擎按主流客户端的方式分层判定：
//
//  1. 目标是 IP   → 查 CN IP 表
//  2. 域名命中 CN 域名表 → 直连
//  3. 未收录域名  → 走代理（白名单策略），或按配置回退到本地解析查 IP
//
// 判定为直连时 gost 由本机 dialer 解析域名（就近 CDN 最优）；
// 判定为代理时 gost 把域名原样交给上游解析（避开本地 DNS 污染）。
package splitd

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
)

// Policy 决定未收录域名如何处理。
type Policy int

const (
	// PolicyProxy 未收录域名走代理（白名单，默认）。
	PolicyProxy Policy = iota
	// PolicyResolve 未收录域名先本地解析，命中 CN IP 则直连。
	PolicyResolve
)

// Options 是引擎配置。
type Options struct {
	// IPListPath 是 CN IP 段列表（每行 CIDR）。
	IPListPath string
	// DomainListPath 是 CN 域名后缀列表（每行一个后缀，如 baidu）。
	DomainListPath string
	// Policy 是未收录域名的处理策略。
	Policy Policy
}

// Engine 保存分流数据并提供判定。
type Engine struct {
	mu      sync.RWMutex
	ipTrie  *cIDRTrie
	domains *domainTrie
	policy  Policy

	// lookup 可在测试中替换，默认 net.LookupIP。
	lookup func(host string) ([]net.IP, error)
}

// New 创建空引擎。
func New(policy Policy) *Engine {
	return &Engine{
		ipTrie:  newCIDRTrie(),
		domains: newDomainTrie(),
		policy:  policy,
		lookup:  net.LookupIP,
	}
}

// Load 载入两个列表（文件不存在时视为空，不报错）。
func (e *Engine) Load(opts Options) (ipCount, domainCount int, err error) {
	e.mu.Lock()
	e.policy = opts.Policy
	e.mu.Unlock()

	ipTrie := newCIDRTrie()
	if opts.IPListPath != "" {
		n, err := loadCIDRs(opts.IPListPath, ipTrie)
		if err != nil {
			return 0, 0, fmt.Errorf("IP 列表: %w", err)
		}
		ipCount = n
	}

	domains := newDomainTrie()
	if opts.DomainListPath != "" {
		n, err := loadDomains(opts.DomainListPath, domains)
		if err != nil {
			return 0, 0, fmt.Errorf("域名列表: %w", err)
		}
		domainCount = n
	}

	e.mu.Lock()
	e.ipTrie, e.domains = ipTrie, domains
	e.mu.Unlock()
	return ipCount, domainCount, nil
}

// Counts 返回已载入的条目数。
func (e *Engine) Counts() (ips, domains int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ipTrie.count(), e.domains.count()
}

// ShouldBypass 判定目标是否直连（true = 直连，false = 走代理）。
//
// target 可以是 "host:port"、"host" 或 "ip:port"。
func (e *Engine) ShouldBypass(target string) bool {
	host := hostOf(target)
	if host == "" {
		return false
	}

	// 1) 目标是 IP：只查 CN IP 表
	if ip := net.ParseIP(host); ip != nil {
		e.mu.RLock()
		hit := e.ipTrie.contains(ip)
		e.mu.RUnlock()
		return hit
	}

	// 2) 域名：先查 CN 域名表
	e.mu.RLock()
	hit := e.domains.contains(host)
	policy := e.policy
	ipTrie := e.ipTrie
	lookup := e.lookup
	e.mu.RUnlock()
	if hit {
		return true
	}

	// 3) 未收录
	if policy == PolicyProxy {
		return false // 白名单：默认走代理
	}
	// 回退：本地解析后查 IP 表
	ips, err := lookup(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ipTrie.contains(ip) {
			return true
		}
	}
	return false
}

// hostOf 从 target 里取出 host 部分（去掉端口与 IPv6 方括号）。
func hostOf(target string) string {
	t := strings.TrimSpace(target)
	if t == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(t); err == nil {
		return strings.Trim(h, "[]")
	}
	return strings.Trim(t, "[]")
}

// loadCIDRs 逐行读取 CIDR（忽略注释与空行）。
func loadCIDRs(path string, dst *cIDRTrie) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	n := 0
	buf := make([]byte, 0, 64*1024)
	readBuf := make([]byte, 64*1024)
	for {
		k, err := f.Read(readBuf)
		buf = append(buf, readBuf[:k]...)
		for {
			i := indexByte(buf, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimSpace(string(buf[:i]))
			buf = buf[i+1:]
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if dst.insertCIDR(line) {
				n++
			}
		}
		if err != nil {
			break
		}
	}
	if line := strings.TrimSpace(string(buf)); line != "" && !strings.HasPrefix(line, "#") {
		if dst.insertCIDR(line) {
			n++
		}
	}
	return n, nil
}

// loadDomains 逐行读取域名后缀（忽略注释、空行、前缀标记）。
func loadDomains(path string, dst *domainTrie) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	n := 0
	buf := make([]byte, 0, 64*1024)
	readBuf := make([]byte, 64*1024)
	for {
		k, err := f.Read(readBuf)
		buf = append(buf, readBuf[:k]...)
		for {
			i := indexByte(buf, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimSpace(string(buf[:i]))
			buf = buf[i+1:]
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if dst.insert(normalizeDomain(line)) {
				n++
			}
		}
		if err != nil {
			break
		}
	}
	if line := strings.TrimSpace(string(buf)); line != "" && !strings.HasPrefix(line, "#") {
		if dst.insert(normalizeDomain(line)) {
			n++
		}
	}
	return n, nil
}

// normalizeDomain 去掉 geosite 风格的前缀（domain:/suffix:/full:）与通配。
func normalizeDomain(s string) string {
	for _, p := range []string{"domain:", "suffix:", "full:", "keyword:"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimPrefix(s, "*.")
	s = strings.TrimPrefix(s, ".")
	return strings.ToLower(strings.TrimSpace(s))
}

// indexByte 返回 b 中 c 的下标（避免每次分配）。
func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
