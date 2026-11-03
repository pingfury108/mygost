package splitd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func newEngine(t *testing.T, ips, domains string, policy Policy) *Engine {
	t.Helper()
	dir := t.TempDir()
	e := New(policy)
	_, _, err := e.Load(Options{
		IPListPath:     writeFile(t, dir, "ip.txt", ips),
		DomainListPath: writeFile(t, dir, "domains.txt", domains),
		Policy:         policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// 纯 IP：只查 IP 表。
func TestShouldBypassIP(t *testing.T) {
	e := newEngine(t, "1.0.1.0/24\n10.0.0.0/8\n", "", PolicyProxy)
	cases := map[string]bool{
		"1.0.1.5:443":     true,
		"10.1.2.3:80":     true,
		"8.8.8.8:443":     false,
		"1.0.2.1:443":     false,
		"2001:db8::1:443": false,
	}
	for target, want := range cases {
		if got := e.ShouldBypass(target); got != want {
			t.Errorf("ShouldBypass(%q) = %v, want %v", target, got, want)
		}
	}
}

// CN 域名：命中即直连（即使它解析到国外 IP，也不做 IP 判定）。
func TestShouldBypassCNDomain(t *testing.T) {
	// 真实 geosite 数据：绝大多数是完整域名（baidu.com），少数是 gTLD（alibaba）
	e := newEngine(t, "8.8.8.0/24\n", "baidu.com\nqq.com\nalibaba\n", PolicyProxy)
	cases := map[string]bool{
		"www.baidu.com:443":      true,
		"baidu.com:443":          true,
		"qq.com:443":             true,
		"www.qq.com:443":         true,
		"x.alibaba:443":          true,  // gTLD 后缀
		"notbaidu.com:443":       false, // 不是后缀命中
		"baidu.com.evil.com:443": false, // 后缀必须落在末尾
		"gateway.icloud.com:443": false,
	}
	for target, want := range cases {
		if got := e.ShouldBypass(target); got != want {
			t.Errorf("ShouldBypass(%q) = %v, want %v", target, got, want)
		}
	}
}

// 未知域名：默认走代理（白名单），即使解析到 CN IP。
func TestUnknownDomainProxyPolicy(t *testing.T) {
	e := newEngine(t, "1.0.1.0/24\n", "baidu.com\n", PolicyProxy)
	// 该域名会解析到 CN IP（模拟），但白名单策略下仍走代理
	e.lookup = func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("1.0.1.9")}, nil
	}
	if e.ShouldBypass("gateway.icloud.com:443") {
		t.Error("unknown domain should go through proxy under PolicyProxy")
	}
}

// 未知域名：可选回退到本地解析查 IP。
func TestUnknownDomainResolvePolicy(t *testing.T) {
	e := newEngine(t, "1.0.1.0/24\n", "", PolicyResolve)
	e.lookup = func(host string) ([]net.IP, error) {
		if host == "cn-site.example" {
			return []net.IP{net.ParseIP("1.0.1.7")}, nil
		}
		return []net.IP{net.ParseIP("8.8.8.8")}, nil
	}
	if !e.ShouldBypass("cn-site.example:443") {
		t.Error("resolved CN IP should bypass under PolicyResolve")
	}
	if e.ShouldBypass("foreign.example:443") {
		t.Error("resolved foreign IP should not bypass")
	}
}

// 空列表不 panic，全部走代理。
func TestEmptyLists(t *testing.T) {
	e := New(PolicyProxy)
	if e.ShouldBypass("example.com:443") {
		t.Error("empty engine should proxy")
	}
	if e.ShouldBypass("") {
		t.Error("empty target should proxy")
	}
}

// 列表文件缺失时按空处理。
func TestMissingFiles(t *testing.T) {
	e := New(PolicyProxy)
	ips, doms, err := e.Load(Options{
		IPListPath:     filepath.Join(t.TempDir(), "nope.txt"),
		DomainListPath: filepath.Join(t.TempDir(), "nope2.txt"),
	})
	if err != nil || ips != 0 || doms != 0 {
		t.Errorf("missing files: %d %d %v", ips, doms, err)
	}
}

// 注释与空行被忽略；重复条目只计一次。
func TestListParsing(t *testing.T) {
	e := newEngine(t, "# comment\n1.0.1.0/24\n\n1.0.1.0/24\n", "# c\ndomain:apple.com\nsuffix:icloud.com\n*.qq.com\n", PolicyProxy)
	ips, doms := e.Counts()
	if ips != 1 {
		t.Errorf("ips = %d, want 1（重复应去重）", ips)
	}
	if doms != 3 {
		t.Errorf("doms = %d, want 3", doms)
	}
	for _, d := range []string{"apple.com", "icloud.com", "qq.com"} {
		if !e.ShouldBypass("x." + d + ":443") {
			t.Errorf("x.%s should be CN domain", d)
		}
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"example.com:443":   "example.com",
		"example.com":       "example.com",
		"1.2.3.4:80":        "1.2.3.4",
		"[2001:db8::1]:443": "2001:db8::1",
		"":                  "",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}
