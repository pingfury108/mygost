package splitd

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-gost/plugin/bypass/proto"
)

// 判定引擎吞吐：单连接内串行 RPC 的每秒调用数。
// 生产中一个 gost 会话就是一个 gRPC stream，这是最贴近真实的路径。
func BenchmarkEngineRPC(b *testing.B) {
	dir := b.TempDir()
	writeRuleFixtures(b, dir)

	eng := New(PolicyProxy)
	if _, _, err := eng.Load(Options{
		IPListPath:     filepath.Join(dir, "geoip.txt"),
		DomainListPath: filepath.Join(dir, "geosite.txt"),
	}); err != nil {
		b.Fatal(err)
	}
	srv := NewServer(eng)
	// 直接调用 Bypass，绕开 gRPC 网络层：先量纯判定
	targets := benchTargets()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			srv.Bypass(nil, &proto.BypassRequest{Addr: targets[i%len(targets)]})
			i++
		}
	})
}

// 并发场景下的判定：模拟多个 gost 会话同时查询。
func BenchmarkEngineParallel(b *testing.B) {
	dir := b.TempDir()
	writeRuleFixtures(b, dir)
	eng := New(PolicyProxy)
	if _, _, err := eng.Load(Options{
		IPListPath:     filepath.Join(dir, "geoip.txt"),
		DomainListPath: filepath.Join(dir, "geosite.txt"),
	}); err != nil {
		b.Fatal(err)
	}
	targets := benchTargets()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			eng.ShouldBypass(targets[i%len(targets)])
			i++
		}
	})
}

// DNS 回退路径（PolicyResolve + 未收录域名）：lookup 失败时立即返回，
// 量的是"解析失败"这个最坏情况的开销（成功路径受本地 DNS 限制，不在此量）。
func BenchmarkEngineResolveFallbackFail(b *testing.B) {
	dir := b.TempDir()
	writeRuleFixtures(b, dir)
	eng := New(PolicyResolve)
	eng.lookup = func(string) ([]net.IP, error) { return nil, errTestLookup }
	targets := benchTargets()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.ShouldBypass(targets[i%len(targets)])
	}
}

var errTestLookup = errors.New("no network in bench")

// benchTargets 是贴近真实流量的目标混合：CN 域名 / 国外域名 / IP。
func benchTargets() []string {
	return []string{
		"www.baidu.com:443", "sp1.baidu.com:443", "v.bilibili.com:443",
		"mail.qq.com:443", "item.jd.com:443", "cn-site-50000.com:443",
		"gateway.icloud.com:443", "api.telegram.org:443",
		"www.gstatic.com:443", "alive.github.com:443",
		"8.8.8.8:443", "1.2.3.4:443",
		"cn-site-99999.com:443", "unknown-foreign.org:443",
	}
}

// benchCIDRs 模拟真实 CN IP 表的分布（节选常见大段）。
var benchCIDRs = []string{
	"1.0.1.0/24", "1.0.2.0/23", "1.0.8.0/21", "1.0.32.0/19",
	"14.0.0.0/8", "27.0.0.0/8", "36.0.0.0/10", "39.0.0.0/8",
	"42.0.0.0/8", "49.0.0.0/8", "58.0.0.0/7", "59.0.0.0/8",
	"60.0.0.0/8", "61.0.0.0/8", "101.0.0.0/8", "106.0.0.0/8",
	"110.0.0.0/7", "112.0.0.0/8", "113.0.0.0/8", "114.0.0.0/8",
	"115.0.0.0/8", "116.0.0.0/8", "117.0.0.0/8", "118.0.0.0/8",
	"119.0.0.0/8", "120.0.0.0/8", "121.0.0.0/8", "122.0.0.0/8",
	"123.0.0.0/8", "124.0.0.0/7", "126.0.0.0/8", "128.0.0.0/8",
	"139.0.0.0/8", "140.0.0.0/8", "144.0.0.0/8", "150.0.0.0/8",
	"153.0.0.0/8", "157.0.0.0/8", "158.0.0.0/8", "159.0.0.0/8",
	"161.0.0.0/8", "162.0.0.0/8", "163.0.0.0/8", "168.0.0.0/8",
	"169.0.0.0/8", "171.0.0.0/8", "175.0.0.0/8", "180.0.0.0/8",
	"182.0.0.0/8", "183.0.0.0/8", "202.0.0.0/8", "203.0.0.0/8",
	"210.0.0.0/8", "211.0.0.0/8", "218.0.0.0/8", "219.0.0.0/8",
	"220.0.0.0/8", "221.0.0.0/8", "222.0.0.0/8", "223.0.0.0/8",
}

func writeRuleFixtures(tb testing.TB, dir string) {
	tb.Helper()
	os.MkdirAll(dir, 0o755)
	// IP 表：模拟真实规模 9662 条（覆盖常见段）
	ipf, _ := os.Create(filepath.Join(dir, "geoip.txt"))
	for _, cidr := range benchCIDRs {
		ipf.WriteString(cidr + "\n")
	}
	ipf.Close()
	// 域名表：110177 条后缀
	df, _ := os.Create(filepath.Join(dir, "geosite.txt"))
	for i := 0; i < 110000; i++ {
		df.WriteString("cn-site-" + itoa(i) + ".com\n")
	}
	df.WriteString("baidu.com\nbilibili.com\nqq.com\ntaobao.com\njd.com\naliyun.com\n")
	df.Close()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
