package profiles

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// NormalizeListen 把用户输入的监听地址规范化成 gost 能接受的形式。
//
// 用户手填时最常见的两种写法都会让 gost 直接失败：
//
//	8085          → gost: listen tcp4: address 8085: missing port in address
//	127.0.0.1     → 同上
//
// 前者意图显然是「监听 8085 端口」，这里统一补成 ":8085"；
// 后者缺端口，无法猜测，直接报错让用户补全。
func NormalizeListen(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil // 空表示交给上层选默认端口
	}
	// 纯数字：当作端口
	if isAllDigits(s) {
		if err := checkPort(s); err != nil {
			return "", err
		}
		return ":" + s, nil
	}
	// unix socket 之类以 / 开头或含 / 的地址：原样保留
	if strings.HasPrefix(s, "/") || strings.Contains(s, "/") {
		return s, nil
	}

	host, port, err := net.SplitHostPort(s)
	if err != nil {
		// 形如 "127.0.0.1"（有 IP 没端口）
		if ip := net.ParseIP(s); ip != nil {
			return "", fmt.Errorf("监听地址 %q 缺少端口，应为 %s:端口 或 :端口", raw, s)
		}
		// 形如 "localhost"（有主机名没端口）
		if !strings.Contains(s, ":") {
			return "", fmt.Errorf("监听地址 %q 缺少端口，应为 :端口 或 主机:端口", raw)
		}
		return "", fmt.Errorf("监听地址 %q 格式不正确: %v", raw, err)
	}
	if err := checkPort(port); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

// checkPort 校验端口字符串。
func checkPort(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("端口 %q 无效，应为 1-65535", s)
	}
	return nil
}

// isAllDigits 报告字符串是否全是十进制数字。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Normalize 规范化配置档里需要校验的字段（目前是监听地址）。
func (p *Profile) Normalize() error {
	if p.Kind == KindCustom {
		return nil // YAML 模式由用户全权负责
	}
	v, err := NormalizeListen(p.Listen)
	if err != nil {
		return err
	}
	p.Listen = v
	return nil
}
