// Package probe 做连通性测试：只走指定配置的本地代理端口，
// 不读系统代理、不读环境变量、不受其它配置影响。
package probe

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Result 是一项测试的结果。
type Result struct {
	Name    string        // 测试项名称
	OK      bool          // 是否通过
	Latency time.Duration // 耗时（失败时为超时值）
	Detail  string        // 说明：状态码、错误原因、地址等
}

// Target 是默认的连通性测试目标：返回 204 且无正文，最能反映链路质量。
// 比 google.com 干净（无重定向、无被劫持风险）。
const Target = "https://www.gstatic.com/generate_204"

// Timeout 是单项测试的超时。
const Timeout = 6 * time.Second

// ViaHTTPProxy 通过本地 HTTP/SOCKS 代理请求 target，验证代理链路。
//
// 关键：使用自建 Transport，Proxy 显式指向 proxyAddr，
// 且 DialContext 不走任何上游代理 —— 因此与系统代理/环境变量完全隔离。
func ViaHTTPProxy(proxyAddr, target string, timeout time.Duration) Result {
	res := Result{Name: "代理链路"}
	pu, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		res.Detail = "无效代理地址"
		return res
	}

	tr := &http.Transport{
		Proxy: http.ProxyURL(pu),
		// 直连本地代理：显式清空环境代理的影响
		DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		// 测试不追求复用
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		// 不跟随重定向：204 才是我们要的答案
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "mygost/probe")

	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Detail = shortErr(err)
		return res
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNoContent:
		res.OK = true
		res.Detail = "204 · " + res.Latency.Round(time.Millisecond).String()
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		// 有响应说明链路通，只是目标不返回 204
		res.OK = true
		res.Detail = fmt.Sprintf("%d · %s", resp.StatusCode, res.Latency.Round(time.Millisecond))
	default:
		res.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return res
}

// Direct 是不经过代理的基线测试，用于对比代理是否更慢。
func Direct(target string, timeout time.Duration) Result {
	res := Result{Name: "直连基线"}
	tr := &http.Transport{
		// 显式禁用代理，避免读到环境变量里的 HTTP_PROXY
		Proxy:       nil,
		DialContext: (&net.Dialer{Timeout: timeout}).DialContext,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		res.Detail = err.Error()
		return res
	}

	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Detail = shortErr(err)
		return res
	}
	defer resp.Body.Close()
	res.OK = resp.StatusCode < 400
	if res.OK {
		res.Detail = fmt.Sprintf("%d · %s", resp.StatusCode, res.Latency.Round(time.Millisecond))
	} else {
		res.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return res
}

// TCPDial 测试某个地址的 TCP 可达性（用于上游隧道）。
func TCPDial(name, addr string, timeout time.Duration) Result {
	res := Result{Name: name}
	if addr == "" {
		res.Detail = "未配置"
		return res
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	res.Latency = time.Since(start)
	if err != nil {
		res.Detail = shortErr(err)
		return res
	}
	conn.Close()
	res.OK = true
	res.Detail = addr + " · " + res.Latency.Round(time.Millisecond).String()
	return res
}

// ListenAddr 把配置里的监听地址转成本机可连接的 host:port。
// ":8027" → "127.0.0.1:8027"；"0.0.0.0:8027" → "127.0.0.1:8027"。
func ListenAddr(listen string) (string, error) {
	if listen == "" {
		return "", fmt.Errorf("未配置监听地址")
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "", fmt.Errorf("无效监听地址 %q", listen)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("无效端口 %q", port)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

// shortErr 把常见网络错误压成一句可读的话。
func shortErr(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "i/o timeout"):
		return "超时"
	case strings.Contains(msg, "connection refused"):
		return "连接被拒绝，代理未运行？"
	case strings.Contains(msg, "no such host"):
		return "域名解析失败"
	case strings.Contains(msg, "certificate"):
		return "TLS 证书错误"
	}
	return msg
}
