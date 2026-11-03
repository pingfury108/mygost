package splitd

import (
	"net"
	"strings"
)

// cIDRTrie 是 IPv4/IPv6 的二进制前缀树（按位插入，按位查找）。
//
// 相比线性遍历 CIDR 列表，查找开销只与地址位数相关（32 或 128 步），
// 因此可以放心载入上万条规则。
type cIDRTrie struct {
	v4   *bitNode
	v6   *bitNode
	size int
}

type bitNode struct {
	child [2]*bitNode
	match bool // 该前缀被标记为命中
}

func newCIDRTrie() *cIDRTrie {
	return &cIDRTrie{v4: &bitNode{}, v6: &bitNode{}}
}

func (t *cIDRTrie) count() int { return t.size }

// insertCIDR 插入一条 CIDR；解析失败返回 false。
func (t *cIDRTrie) insertCIDR(s string) bool {
	ip, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		// 允许裸 IP（等价于 /32 或 /128）
		if ip := net.ParseIP(s); ip != nil {
			ones, _ := bitsOf(ip)
			return t.insertIP(ip, ones)
		}
		return false
	}
	ones, bits := ipnet.Mask.Size()
	return t.insertIP(ip, ones, bits)
}

func bitsOf(ip net.IP) (ones, bits int) {
	if ip.To4() != nil {
		return 32, 32
	}
	return 128, 128
}

// insertIP 插入前缀。
func (t *cIDRTrie) insertIP(ip net.IP, ones int, bits ...int) bool {
	isV4 := ip.To4() != nil
	var raw []byte
	var total int
	root := t.v6
	if isV4 {
		raw = ip.To4()
		total = 32
		root = t.v4
	} else {
		raw = ip.To16()
		total = 128
	}
	if len(bits) > 0 && bits[0] > 0 {
		total = bits[0]
	}
	if ones > total {
		ones = total
	}

	node := root
	for i := 0; i < ones; i++ {
		bit := bitAt(raw, i)
		if node.child[bit] == nil {
			node.child[bit] = &bitNode{}
		}
		node = node.child[bit]
	}
	if node.match {
		return false
	}
	node.match = true
	t.size++
	return true
}

// contains 查询 IP 是否命中任一前缀。
func (t *cIDRTrie) contains(ip net.IP) bool {
	var raw []byte
	root := t.v6
	if v4 := ip.To4(); v4 != nil {
		raw, root = v4, t.v4
	} else {
		raw = ip.To16()
	}
	if raw == nil {
		return false
	}
	bits := len(raw) * 8

	node := root
	if node.match {
		return true
	}
	for i := 0; i < bits; i++ {
		node = node.child[bitAt(raw, i)]
		if node == nil {
			return false
		}
		if node.match {
			return true
		}
	}
	return false
}

// bitAt 返回第 i 位（大端位序）。
func bitAt(raw []byte, i int) int {
	byteIdx := i / 8
	if byteIdx >= len(raw) {
		return 0
	}
	shift := 7 - uint(i%8)
	return int(raw[byteIdx]>>shift) & 1
}

// domainTrie 是域名后缀树（按标签反转插入）。
//
// "www.baidu.com" 被拆成 [com, baidu, www] 逐层下探；任一节点标记为命中
// 即表示该后缀被收录（等价于 geosite 的 domain_suffix 语义）。
type domainTrie struct {
	root *labelNode
	size int
}

type labelNode struct {
	children map[string]*labelNode
	match    bool
}

func newDomainTrie() *domainTrie {
	return &domainTrie{root: &labelNode{children: map[string]*labelNode{}}}
}

func (t *domainTrie) count() int { return t.size }

// insert 插入一个域名后缀，返回是否为新增。
func (t *domainTrie) insert(domain string) bool {
	domain = trimDot(domain)
	if domain == "" {
		return false
	}
	labels := splitLabels(domain)
	if len(labels) == 0 {
		return false
	}

	node := t.root
	for i := len(labels) - 1; i >= 0; i-- {
		label := labels[i]
		if node.children == nil {
			node.children = map[string]*labelNode{}
		}
		next := node.children[label]
		if next == nil {
			next = &labelNode{}
			node.children[label] = next
		}
		node = next
	}
	if node.match {
		return false
	}
	node.match = true
	t.size++
	return true
}

// contains 判断 host 是否命中任一后缀。
func (t *domainTrie) contains(host string) bool {
	host = strings.ToLower(trimDot(host))
	if host == "" || t.root == nil {
		return false
	}
	labels := splitLabels(host)
	node := t.root
	if node.match {
		return true
	}
	for i := len(labels) - 1; i >= 0; i-- {
		node = node.children[labels[i]]
		if node == nil {
			return false
		}
		if node.match {
			return true // 命中某个后缀
		}
	}
	return false
}

func splitLabels(domain string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(domain); i++ {
		if i == len(domain) || domain[i] == '.' {
			if i > start {
				out = append(out, domain[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func trimDot(s string) string {
	for len(s) > 0 && s[0] == '.' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}
