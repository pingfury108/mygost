package profiles

import (
	"fmt"
	"net/url"
	"strings"
)

// ConnectorTypes 是 gost 支持的转发链节点类型（connector）。
// 顺序按使用频率排列，UI 下拉列表直接使用。
var ConnectorTypes = []string{
	"socks5", "http", "sshd", "ssh", "ss", "relay", "forward", "sni",
	"socks4", "socks4a", "socks", "http2", "tunnel", "tcp", "unix",
	"direct", "masque", "router", "serial", "variant",
}

// Node 是一个 gost 节点，对应 URL 形式 `type://user:pass@host:port`。
type Node struct {
	Type string // connector / handler 类型，可与地址分开
	Addr string // host:port
	User string
	Pass string
}

// ParseNode 解析单个 gost 节点，接受以下形式：
//
//	sshd://user:pass@1.2.3.4:8081
//	-F sshd://user:pass@1.2.3.4:8081
//	-L=:8080
//	:8080
//
// 只有地址时 Type 为空。
func ParseNode(raw string) (Node, error) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`)
	for _, p := range []string{"-F=", "-L=", "-F ", "-L "} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return Node{}, fmt.Errorf("内容为空")
	}
	if !strings.Contains(s, "://") {
		return Node{Addr: s}, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return Node{}, fmt.Errorf("无法解析 %q: %w", raw, err)
	}
	n := Node{Type: u.Scheme, Addr: u.Host}
	if n.Addr == "" {
		// http://:8080 这类没有主机的写法
		n.Addr = u.Path
	}
	if u.User != nil {
		n.User = u.User.Username()
		n.Pass, _ = u.User.Password()
	}
	if n.Addr == "" {
		return Node{}, fmt.Errorf("缺少地址")
	}
	return n, nil
}

// ParseCommand 解析 gost 启动命令，取出本地服务（-L）与转发链节点（-F）。
//
//	gost -L=:8080 -F=sshd://user:pass@1.2.3.4:8081
//
// 也接受只给 -F 或只给链接的情况。hasChain 表示命令中带有 -F。
func ParseCommand(s string) (listen, chain Node, hasChain bool, err error) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) == 0 {
		return Node{}, Node{}, false, fmt.Errorf("命令为空")
	}

	pick := func(flag string) (Node, bool, error) {
		for i, f := range fields {
			var v string
			switch {
			case f == flag:
				if i+1 >= len(fields) {
					return Node{}, false, fmt.Errorf("%s 缺少值", flag)
				}
				v = fields[i+1]
			case strings.HasPrefix(f, flag+"="):
				v = strings.TrimPrefix(f, flag+"=")
			default:
				continue
			}
			n, e := ParseNode(v)
			return n, true, e
		}
		return Node{}, false, nil
	}

	listen, _, err = pick("-L")
	if err != nil {
		return Node{}, Node{}, false, err
	}
	chain, hasChain, err = pick("-F")
	if err != nil {
		return Node{}, Node{}, false, err
	}
	if listen.Addr == "" && chain.Addr == "" {
		// 没写 -L/-F：把整串当作单个节点尝试
		n, e := ParseNode(strings.Join(fields, " "))
		if e != nil {
			return Node{}, Node{}, false, e
		}
		return n, Node{}, false, nil
	}
	return listen, chain, hasChain, nil
}
