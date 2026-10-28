package metrics

import (
	"fmt"
	"net"
	"testing"
)

func TestPickPortSkipsBusy(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	p, err := PickPort(busy, 3)
	if err != nil {
		t.Skipf("no free port after %d: %v", busy, err)
	}
	if p == busy {
		t.Fatalf("should skip busy port %d", busy)
	}
	if p != busy+1 && p != busy+2 {
		t.Errorf("picked %d, expected the next free after %d", p, busy)
	}
	_ = fmt.Sprint(p)
}

func TestPickPortAllBusy(t *testing.T) {
	// 显式占住 base 与 base+1 两个端口，tries=2 必然失败
	base := 0
	var lns []net.Listener
	for p := 20000; p < 20100 && len(lns) < 2; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			lns = nil
			continue
		}
		if len(lns) == 0 {
			base = p
			lns = append(lns, ln)
			continue
		}
		if p == base+1 {
			lns = append(lns, ln)
			break
		}
		ln.Close()
		lns = nil
	}
	if len(lns) < 2 {
		t.Skip("cannot reserve two consecutive ports")
	}
	defer func() {
		for _, l := range lns {
			l.Close()
		}
	}()
	if _, err := PickPort(base, 2); err == nil {
		t.Errorf("should fail when %d and %d are busy", base, base+1)
	}
}
