package profiles

import "testing"

func TestParseNodeNodeURL(t *testing.T) {
	n, err := ParseNode("sshd://gost:secret@149.129.41.181:8081")
	if err != nil {
		t.Fatal(err)
	}
	want := Node{Type: "sshd", Addr: "149.129.41.181:8081", User: "gost", Pass: "secret"}
	if n != want {
		t.Errorf("got %+v, want %+v", n, want)
	}
}

func TestParseNodeWithFlagPrefix(t *testing.T) {
	for _, raw := range []string{
		"-F sshd://gost:secret@1.2.3.4:8081",
		"-F=sshd://gost:secret@1.2.3.4:8081",
	} {
		n, err := ParseNode(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if n.Type != "sshd" || n.Addr != "1.2.3.4:8081" || n.User != "gost" {
			t.Errorf("%q => %+v", raw, n)
		}
	}
}

func TestParseNodeAddrOnly(t *testing.T) {
	for _, raw := range []string{":8080", "-L=:8080", "-L :8080", "http://:8080"} {
		n, err := ParseNode(raw)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if n.Addr != ":8080" {
			t.Errorf("%q => addr %q", raw, n.Addr)
		}
	}
}

func TestParseNodeErrors(t *testing.T) {
	for _, raw := range []string{"", "   ", "sshd://"} {
		if _, err := ParseNode(raw); err == nil {
			t.Errorf("%q should fail", raw)
		}
	}
}

// 用户实际使用的命令形态。
func TestParseCommand(t *testing.T) {
	listen, chain, hasChain, err := ParseCommand("gost -L=:8080 -F sshd://gost:xxx@149.129.41.181:8081")
	if err != nil {
		t.Fatal(err)
	}
	if listen.Addr != ":8080" {
		t.Errorf("listen addr %q", listen.Addr)
	}
	if !hasChain {
		t.Fatal("chain should be present")
	}
	if chain.Type != "sshd" || chain.Addr != "149.129.41.181:8081" || chain.User != "gost" || chain.Pass != "xxx" {
		t.Errorf("chain %+v", chain)
	}
}

func TestParseCommandOnlyChain(t *testing.T) {
	_, chain, hasChain, err := ParseCommand("-F=sshd://u:p@h:1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasChain || chain.Type != "sshd" || chain.Addr != "h:1" {
		t.Errorf("chain %+v hasChain=%v", chain, hasChain)
	}
}

func TestParseCommandOnlyURL(t *testing.T) {
	listen, _, hasChain, err := ParseCommand("sshd://u:p@h:1")
	if err != nil {
		t.Fatal(err)
	}
	if hasChain {
		t.Error("no -F should report no chain")
	}
	if listen.Type != "sshd" {
		t.Errorf("listen %+v", listen)
	}
}

func TestConnectorTypesNotEmpty(t *testing.T) {
	if len(ConnectorTypes) < 5 {
		t.Fatal("connector list too short")
	}
	for _, c := range ConnectorTypes {
		if c == "" {
			t.Error("empty connector type")
		}
	}
}
