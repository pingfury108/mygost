package probe

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 代理链路测试必须只经过给定的代理，不受环境变量影响。
func TestViaHTTPProxyUsesGivenProxyOnly(t *testing.T) {
	// 目标站：返回 204
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	// 假代理：记录收到的请求，并代为请求目标
	var seen int
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()

	// 设置一个会失败的环境代理，验证它被忽略
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	addr := strings.TrimPrefix(proxy.URL, "http://")
	res := ViaHTTPProxy(addr, "http://example.test/generate_204", 3*time.Second)
	if !res.OK {
		t.Fatalf("probe failed: %+v", res)
	}
	if seen == 0 {
		t.Error("proxy was not used")
	}
}

// 代理未运行时，测试应明确失败而不是静默通过。
func TestViaHTTPProxyFailsWhenProxyDown(t *testing.T) {
	// 找一个空闲端口后关闭，确保无人监听
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	res := ViaHTTPProxy(addr, "http://example.test/", 800*time.Millisecond)
	if res.OK {
		t.Fatalf("should fail: %+v", res)
	}
	if res.Detail == "" {
		t.Error("failure needs a reason")
	}
}

// 直连基线显式禁用代理，即使环境里有 HTTP_PROXY 也不使用。
func TestDirectIgnoresEnvProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	res := Direct(srv.URL, 3*time.Second)
	if !res.OK {
		t.Fatalf("direct should bypass env proxy: %+v", res)
	}
}

// TCPDial：监听中的地址通过，未监听的失败。
func TestTCPDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if res := TCPDial("上游", ln.Addr().String(), time.Second); !res.OK {
		t.Errorf("listening addr should pass: %+v", res)
	}
	if res := TCPDial("上游", "", time.Second); res.OK || res.Detail != "未配置" {
		t.Errorf("empty addr: %+v", res)
	}
}

func TestListenAddr(t *testing.T) {
	cases := map[string]string{
		":8027":          "127.0.0.1:8027",
		"0.0.0.0:8027":   "127.0.0.1:8027",
		"127.0.0.1:9000": "127.0.0.1:9000",
		"[::]:8000":      "127.0.0.1:8000",
	}
	for in, want := range cases {
		got, err := ListenAddr(in)
		if err != nil || got != want {
			t.Errorf("ListenAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "garbage", ":abc"} {
		if _, err := ListenAddr(bad); err == nil {
			t.Errorf("ListenAddr(%q) should fail", bad)
		}
	}
}
