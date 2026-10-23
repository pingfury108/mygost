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
