package profiles

import "testing"

// 监听地址规范化：手填端口是常见错误来源。
func TestNormalizeListen(t *testing.T) {
	ok := map[string]string{
		"8085":           ":8085",
		":8085":          ":8085",
		"  8085  ":       ":8085",
		"127.0.0.1:8085": "127.0.0.1:8085",
		"0.0.0.0:1080":   "0.0.0.0:1080",
		"[::1]:8080":     "[::1]:8080",
		"":               "",
	}
	for in, want := range ok {
		got, err := NormalizeListen(in)
		if err != nil {
			t.Errorf("NormalizeListen(%q) 报错: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeListen(%q) = %q, want %q", in, got, want)
		}
	}

	bad := []string{"127.0.0.1", "localhost", "abc", ":0", "70000", ":70000"}
	for _, in := range bad {
		if got, err := NormalizeListen(in); err == nil {
			t.Errorf("NormalizeListen(%q) 应报错，得到 %q", in, got)
		}
	}
}

// 保存时规范化：8085 会存成 :8085，非法值直接拒绝。
func TestSaveNormalizesListen(t *testing.T) {
	s, err := LoadStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &Profile{Name: "p", Kind: KindProxy, Listen: "8085", Proto: "http"}
	if _, err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	got := s.Items()
	if len(got) != 1 || got[0].Listen != ":8085" {
		t.Errorf("保存后监听地址应规范化为 :8085，实际 %q", got[0].Listen)
	}

	bad := &Profile{Name: "bad", Kind: KindProxy, Listen: "127.0.0.1", Proto: "http"}
	if _, err := s.Save(bad); err == nil {
		t.Error("缺少端口的地址应被拒绝")
	}
}
