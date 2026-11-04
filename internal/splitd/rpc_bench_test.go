package splitd

import (
	"context"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/go-gost/plugin/bypass/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// gRPC 全链路基准：真实 listen + client conn，含序列化与网络往返。
// 这是 gost 每个请求的真实成本。
func BenchmarkGRPCFull(b *testing.B) {
	dir := b.TempDir()
	writeRuleFixtures(b, dir)
	eng := New(PolicyProxy)
	if _, _, err := eng.Load(Options{
		IPListPath:     filepath.Join(dir, "geoip.txt"),
		DomainListPath: filepath.Join(dir, "geosite.txt"),
	}); err != nil {
		b.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer ln.Close()
	gs := grpc.NewServer()
	proto.RegisterBypassServer(gs, NewServer(eng))
	go gs.Serve(ln)
	defer gs.Stop()

	conn, err := grpc.NewClient(ln.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	client := proto.NewBypassClient(conn)

	targets := benchTargets()
	ctx := context.Background()

	// 单连接串行
	b.Run("serial", func(b *testing.B) {
		i := 0
		b.ReportAllocs()
		b.ResetTimer()
		for i < b.N {
			if _, err := client.Bypass(ctx, &proto.BypassRequest{Addr: targets[i%len(targets)]}); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})

	// 4 并发（模拟多个 gost 会话）
	b.Run("concurrent4", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		var wg sync.WaitGroup
		per := b.N / 4
		for w := 0; w < 4; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := 0; i < per; i++ {
					t := targets[(w*per+i)%len(targets)]
					if _, err := client.Bypass(ctx, &proto.BypassRequest{Addr: t}); err != nil {
						b.Error(err)
						return
					}
				}
			}(w)
		}
		wg.Wait()
	})
}

// 规则载入耗时：启动时的成本，只发生一次。
func BenchmarkLoad(b *testing.B) {
	dir := b.TempDir()
	writeRuleFixtures(b, dir)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng := New(PolicyProxy)
		if _, _, err := eng.Load(Options{
			IPListPath:     filepath.Join(dir, "geoip.txt"),
			DomainListPath: filepath.Join(dir, "geosite.txt"),
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// 内存占用报告：载入 9662 IP + 110177 域名后的堆增量。
func TestEngineMemoryFootprint(t *testing.T) {
	dir := t.TempDir()
	writeRuleFixtures(t, dir)

	var ms1, ms2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms1)

	eng := New(PolicyProxy)
	if _, _, err := eng.Load(Options{
		IPListPath:     filepath.Join(dir, "geoip.txt"),
		DomainListPath: filepath.Join(dir, "geosite.txt"),
	}); err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	runtime.ReadMemStats(&ms2)
	delta := int64(ms2.TotalAlloc) - int64(ms1.TotalAlloc)
	t.Logf("载入分配 %.1f MB（ HeapAlloc Δ %+.2f MB）",
		float64(delta)/1e6,
		float64(int64(ms2.HeapAlloc)-int64(ms1.HeapAlloc))/1e6)
	if delta > 120*1e6 {
		t.Errorf("载入分配 %.1f MB 超预期（<120MB）", float64(delta)/1e6)
	}
}
