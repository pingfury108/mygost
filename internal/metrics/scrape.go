// Package metrics 从 gost 的 Prometheus 端点抓取流量与连接数据。
package metrics

import (
	"bufio"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 关注的指标名（gost v3 的 Prometheus 命名）。
const (
	MetricInputBytes  = "gost_service_transfer_input_bytes_total"
	MetricOutputBytes = "gost_service_transfer_output_bytes_total"
	MetricRequests    = "gost_service_requests_total"
	MetricInflight    = "gost_service_requests_inflight"
	MetricErrors      = "gost_service_handler_errors_total"
)

// Snapshot 是一次抓取的累计值。
type Snapshot struct {
	Input  float64 // 累计输入字节（代理→客户端方向按 gost 定义）
	Output float64 // 累计输出字节
	Reqs   float64 // 累计请求数
	Errors float64 // 累计错误数
	At     time.Time
}

// Client 抓取 gost 的 metrics 端点。
type Client struct {
	mu   sync.Mutex
	addr string // host:port
	http *http.Client
}

// NewClient 创建抓取客户端；addr 形如 "127.0.0.1:9100"。
func NewClient(addr string) *Client {
	return &Client{
		addr: addr,
		http: &http.Client{
			Timeout: 3 * time.Second,
			// metrics 是本机端点，绝不能走任何代理
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

// SetAddr 更新端点（gost 重启后端口可能变化）。
func (c *Client) SetAddr(addr string) {
	c.mu.Lock()
	c.addr = addr
	c.mu.Unlock()
}

// Addr 返回当前端点。
func (c *Client) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.addr
}

// Scrape 抓一次累计值。端点不可用时返回错误（调用方跳过本次采样）。
func (c *Client) Scrape() (Snapshot, error) {
	addr := c.Addr()
	if addr == "" {
		return Snapshot{}, ErrNoEndpoint
	}
	resp, err := c.http.Get("http://" + addr + "/metrics")
	if err != nil {
		return Snapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, ErrBadStatus
	}
	snap, err := Parse(resp.Body)
	if err != nil {
		return Snapshot{}, err
	}
	snap.At = time.Now()
	return snap, nil
}

// Parse 解析 Prometheus 文本，按指标名求和（忽略标签）。
func Parse(r io.Reader) (Snapshot, error) {
	var s Snapshot
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, val, ok := splitSample(line)
		if !ok {
			continue
		}
		switch name {
		case MetricInputBytes:
			s.Input += val
		case MetricOutputBytes:
			s.Output += val
		case MetricRequests:
			s.Reqs += val
		case MetricErrors:
			s.Errors += val
		}
	}
	return s, sc.Err()
}

// splitSample 拆出指标名与数值：name{label="v"} 123
func splitSample(line string) (name string, val float64, ok bool) {
	// 指标名到第一个 { 或空格为止
	end := len(line)
	if i := strings.IndexAny(line, "{ "); i >= 0 {
		end = i
	}
	name = line[:end]
	if name == "" {
		return "", 0, false
	}
	sp := strings.LastIndexByte(line, ' ')
	if sp < 0 || sp == len(line)-1 {
		return "", 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(line[sp+1:]), 64)
	if err != nil {
		return "", 0, false
	}
	return name, v, true
}

// ErrNoEndpoint 表示还没拿到 metrics 端点。
var ErrNoEndpoint = errString("metrics 端点未就绪")

// ErrBadStatus 表示端点返回非 200。
var ErrBadStatus = errString("metrics 端点返回异常状态")

type errString string

func (e errString) Error() string { return string(e) }
