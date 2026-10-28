package profiles

import (
	"os"
	"strings"
	"testing"
)

func TestBuildYAMLProxy(t *testing.T) {
	p := &Profile{
		Kind: KindProxy, Listen: ":8080", Proto: "http",
		User: "u", Password: "p",
		UpEnabled: true, UpProto: "socks5", UpAddr: "1.2.3.4:1080",
	}
	y, err := p.BuildYAML()
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	for _, want := range []string{
		`addr: ":8080"`, "type: http", "chain: chain-0",
		"username: \"u\"", "type: socks5", `addr: "1.2.3.4:1080"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("yaml missing %q:\n%s", want, s)
		}
	}
}

func TestBuildYAMLForwardMultiTargets(t *testing.T) {
	p := &Profile{Kind: KindForward, Listen: ":2222", FwdProto: "tcp", Targets: "1.1.1.1:22\n2.2.2.2:22"}
	y, err := p.BuildYAML()
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	if !strings.Contains(s, "target-0") || !strings.Contains(s, "target-1") {
		t.Errorf("expected two targets:\n%s", s)
	}
}

func TestBuildYAMLForwardRequiresTarget(t *testing.T) {
	p := &Profile{Kind: KindForward, Listen: ":2222", Targets: " "}
	if _, err := p.BuildYAML(); err == nil {
		t.Error("expected error for empty targets")
	}
}

func TestBuildYAMLProxyRequiresUpstream(t *testing.T) {
	p := &Profile{Kind: KindProxy, Listen: ":8080", UpEnabled: true, UpAddr: " "}
	if _, err := p.BuildYAML(); err == nil {
		t.Error("expected error for empty upstream addr")
	}
}

func TestStoreSaveLoadRoundtrip(t *testing.T) {
	s, err := LoadStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &Profile{Name: "a", Kind: KindProxy, Listen: ":80", Proto: "http"}
	yml, err := s.Save(p)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" {
		t.Fatal("ID should be assigned")
	}
	if got := s.Get(p.ID); got == nil || got.Name != "a" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	data, err := os.ReadFile(yml)
	if err != nil || !strings.Contains(string(data), `":80"`) {
		t.Fatalf("yml not written: %q %v", string(data), err)
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if s.Get(p.ID) != nil {
		t.Fatal("should be deleted")
	}
}

// sshd 这类 connector 必须搭配同名 dialer，且认证写在 dialer 上，
// 否则 gost 报 "ssh: invalid connection"（dialer 只给了裸 TCP）。
func TestBuildYAMLSSHDNode(t *testing.T) {
	p := &Profile{
		Kind: KindProxy, Listen: ":8026", Proto: "http",
		UpEnabled: true, UpProto: "sshd",
		UpAddr: "1.2.3.4:8081", UpUser: "gost", UpPass: "xxx",
	}
	y, err := p.BuildYAML()
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	if !strings.Contains(s, "connector:\n              type: sshd") {
		t.Errorf("connector type wrong:\n%s", s)
	}
	if !strings.Contains(s, "dialer:\n              type: sshd") {
		t.Errorf("dialer must mirror sshd:\n%s", s)
	}
	// 认证应在 dialer 下（紧跟 dialer 的 type），connector 下不应有 auth
	di := strings.Index(s, "dialer:")
	if !strings.Contains(s[di:], "auth:") {
		t.Errorf("auth should live under dialer:\n%s", s)
	}
	ci := strings.Index(s, "connector:")
	if strings.Contains(s[ci:di], "auth:") {
		t.Errorf("connector should not carry auth for sshd:\n%s", s)
	}
}

// 普通 connector（如 socks5）保持 dialer=tcp、认证在 connector 上。
func TestBuildYAMLProxyNodeKeepsTCPDialer(t *testing.T) {
	p := &Profile{
		Kind: KindProxy, Listen: ":8026", UpEnabled: true,
		UpProto: "socks5", UpAddr: "1.2.3.4:1080", UpUser: "u", UpPass: "p",
	}
	y, err := p.BuildYAML()
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	if !strings.Contains(s, "dialer:\n              type: tcp") {
		t.Errorf("socks5 should keep tcp dialer:\n%s", s)
	}
	di := strings.Index(s, "dialer:")
	if strings.Contains(s[di:], "auth:") {
		t.Errorf("socks5 auth belongs to connector, not dialer:\n%s", s)
	}
}

func TestDialerFor(t *testing.T) {
	cases := map[string]string{
		"sshd":   "sshd",
		"ssh":    "ssh",
		"tls":    "tls",
		"socks5": "tcp",
		"http":   "tcp",
		"ss":     "tcp",
		"ssu":    "udp",
		"":       "tcp",
	}
	for in, want := range cases {
		if got := DialerFor(in); got != want {
			t.Errorf("DialerFor(%q) = %q, want %q", in, got, want)
		}
	}
}

// 分流开启时：hop 挂 split-direct，附加私网直连与 geo 插件两个 bypass。
func TestBuildYAMLSplitEnabled(t *testing.T) {
	p := &Profile{
		Kind: KindProxy, Listen: ":8026", Proto: "http",
		UpEnabled: true, UpProto: "sshd", UpAddr: "1.2.3.4:8081",
		UpUser: "gost", UpPass: "xxx",
	}
	y, err := p.BuildYAMLSplit(SplitConfig{
		Enabled:         true,
		PluginAddr:      "127.0.0.1:8100",
		PrivateMatchers: []string{"10.0.0.0/8", "192.168.0.0/16"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(y)
	for _, want := range []string{
		"bypass: split-direct",
		"name: split-private",
		"10.0.0.0/8",
		"name: split-geo",
		"whitelist: true",
		"type: grpc",
		"addr: 127.0.0.1:8100",
		"name: split-direct",
		"bypasses:",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("yaml missing %q:\n%s", want, s)
		}
	}
	// sshd 的 dialer/auth 修正不能被破坏
	if !strings.Contains(s, "dialer:\n              type: sshd") {
		t.Errorf("sshd dialer lost:\n%s", s)
	}
}

// 分流关闭时不附加任何 bypasses。
func TestBuildYAMLSplitDisabled(t *testing.T) {
	p := &Profile{Kind: KindProxy, Listen: ":8026", Proto: "http"}
	y, err := p.BuildYAMLSplit(SplitConfig{Enabled: false, PluginAddr: "127.0.0.1:8100"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(y), "bypasses:") {
		t.Errorf("split disabled but bypasses present:\n%s", y)
	}
}

// metrics 段：无论是否分流都应写入。
func TestBuildYAMLIncludesMetrics(t *testing.T) {
	p := &Profile{Kind: KindProxy, Listen: ":8026", Proto: "http"}
	y, err := p.BuildYAMLSplit(SplitConfig{MetricsAddr: "127.0.0.1:9100"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), `metrics:
  addr: "127.0.0.1:9100"`) {
		t.Errorf("metrics missing:\n%s", y)
	}

	// 空地址不写入
	y2, _ := p.BuildYAMLSplit(SplitConfig{})
	if strings.Contains(string(y2), "metrics:") {
		t.Errorf("metrics should be omitted when addr empty:\n%s", y2)
	}

	// 分流开启时 metrics 与 bypasses 并存
	y3, _ := p.BuildYAMLSplit(SplitConfig{
		Enabled: true, PluginAddr: "127.0.0.1:8100",
		PrivateMatchers: []string{"10.0.0.0/8"},
		MetricsAddr:     "127.0.0.1:9100",
	})
	s3 := string(y3)
	if !strings.Contains(s3, "bypasses:") || !strings.Contains(s3, "metrics:") {
		t.Errorf("split + metrics should coexist:\n%s", s3)
	}
}
