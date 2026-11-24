package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mygost/internal/metrics"
	"mygost/internal/proc"
	"mygost/internal/reqlog"
)

// 临时端到端基准：MYGOST_BENCH=1 go test -run TestLocalThroughputComparison -v .
// 使用独立回环端口与临时目录，不接触正在运行的实例。
func TestLocalThroughputComparison(t *testing.T) {
	if os.Getenv("MYGOST_BENCH") == "" {
		t.Skip("set MYGOST_BENCH=1")
	}
	home, _ := os.UserHomeDir()
	bin := filepath.Join(home, "Library", "Application Support", "mygost", "bin", "gost")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("gost not installed")
	}

	newTarget := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	plainTarget, managedTarget := newTarget(), newTarget()
	defer plainTarget.Close()
	defer managedTarget.Close()

	newUpstream := func() (string, *exec.Cmd) {
		addr := freeAddr(t)
		cmd := exec.Command(bin, "-L", "socks5://"+addr)
		cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waitTCP(t, addr)
		return addr, cmd
	}
	plainUpAddr, plainUp := newUpstream()
	managedUpAddr, managedUp := newUpstream()
	defer killWait(plainUp)
	defer killWait(managedUp)

	dir := t.TempDir()
	level := os.Getenv("MYGOST_BENCH_LOG")
	if level == "" {
		level = "debug"
	}
	plainAddr, managedAddr := freeAddr(t), freeAddr(t)
	plainMetrics, err := metrics.PickPort(29190, 20)
	if err != nil {
		t.Fatal(err)
	}
	managedMetrics, err := metrics.PickPort(plainMetrics+20, 20)
	if err != nil {
		t.Fatal(err)
	}
	plainYML := filepath.Join(dir, "plain.yml")
	managedYML := filepath.Join(dir, "managed.yml")
	if err := os.WriteFile(plainYML, benchYAML(plainAddr, plainUpAddr, plainMetrics, level), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedYML, benchYAML(managedAddr, managedUpAddr, managedMetrics, level), 0o644); err != nil {
		t.Fatal(err)
	}

	plain := exec.Command(bin, "-C", plainYML)
	plain.Stdout, plain.Stderr = io.Discard, io.Discard
	if err := plain.Start(); err != nil {
		t.Fatal(err)
	}
	defer killWait(plain)
	waitTCP(t, plainAddr)

	a := newTestApp(t)
	a.reqs = reqlog.New(20000)
	a.initRequestStore(t.TempDir())
	a.pm.SetOnLine(a.feedRequest)
	if err := a.pm.Start(bin, managedYML); err != nil {
		t.Fatal(err)
	}
	defer func() {
		a.stopMetrics()
		a.pm.StopAndWait(3 * time.Second)
		a.flushRequests()
	}()
	if err := a.pm.WaitStable(800 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitTCP(t, managedAddr)
	a.startMetrics(managedMetrics)

	// 真实应用每 500ms 落盘一次；基准里也跑同样的周期，
	// 否则测出的只是解析开销，漏掉写盘竞争。
	flushStop := make(chan struct{})
	flushExited := make(chan struct{})
	go func() {
		defer close(flushExited)
		tk := time.NewTicker(requestFlushInterval)
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				a.reqs.Flush()
			case <-flushStop:
				return
			}
		}
	}()
	defer func() {
		close(flushStop)
		<-flushExited
	}()

	time.Sleep(300 * time.Millisecond)
	rps1, p50a, p95a, n1 := runProxyBench(t, plainAddr, plainTarget.URL+"/", 2*time.Second, 32)
	// 每请求都会消耗临时端口；紧接着跑第二侧会因端口耗尽产生假回归。
	t.Logf("等待临时端口回收…")
	time.Sleep(36 * time.Second)
	rps2, p50b, p95b, n2 := runProxyBench(t, managedAddr, managedTarget.URL+"/", 2*time.Second, 32)
	t.Logf("plain   : %7.1f req/s  p50=%v p95=%v n=%d", rps1, p50a, p95a, n1)
	t.Logf("mygost  : %7.1f req/s  p50=%v p95=%v n=%d", rps2, p50b, p95b, n2)
	t.Logf("parsed  : %d requests", a.reqs.Len())
	t.Logf("ratio   : %.1f%%", 100*rps2/rps1)
}

func benchYAML(listen, upstream string, metricsPort int, level string) []byte {
	return []byte(fmt.Sprintf(`services:
  - name: mygost
    addr: %q
    handler:
      type: http
    listener:
      type: tcp
chains:
  - name: chain-0
    hops:
      - name: hop-0
        nodes:
          - name: node-0
            addr: %q
            connector:
              type: socks5
            dialer:
              type: tcp
metrics:
  addr: "127.0.0.1:%d"
log:
  level: %s
`, listen, upstream, metricsPort, level))
}

func runProxyBench(t *testing.T, proxyAddr, target string, d time.Duration, workers int) (float64, time.Duration, time.Duration, int) {
	t.Helper()
	pu, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		t.Fatal(err)
	}
	tr := &http.Transport{
		Proxy:               http.ProxyURL(pu),
		MaxIdleConns:        workers * 2,
		MaxIdleConnsPerHost: workers * 2,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	defer tr.CloseIdleConnections()

	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	var count atomic.Int64
	var failed atomic.Int64
	lat := make(chan time.Duration, workers*1000)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
				t0 := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						failed.Add(1)
					}
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				lat <- time.Since(t0)
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	close(lat)
	elapsed := time.Since(start)
	all := make([]time.Duration, 0, count.Load())
	for v := range lat {
		all = append(all, v)
	}
	if len(all) == 0 {
		t.Fatalf("no requests: failed=%d", failed.Load())
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	p := func(q float64) time.Duration { return all[int(float64(len(all)-1)*q)] }
	rps := float64(count.Load()) / elapsed.Seconds()
	if failed.Load() > 0 {
		t.Logf("failed=%d", failed.Load())
	}
	return rps, p(0.50), p(0.95), int(count.Load())
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", addr)
}

func killWait(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

var _ = proc.Running
