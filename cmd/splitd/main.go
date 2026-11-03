// splitd 是 mygost 的分流插件：按域名表与 IP 表判定请求该直连还是走代理。
//
// 它实现 gost 的 bypass gRPC 插件协议，由 mygost 启动并托管。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mygost/internal/splitd"
)

func main() {
	addr := flag.String("listen", "127.0.0.1:8100", "gRPC 监听地址")
	ipList := flag.String("ip-list", "", "CN IP 段列表文件（每行 CIDR）")
	domainList := flag.String("domain-list", "", "CN 域名后缀列表文件（每行一个）")
	policy := flag.String("policy", "proxy", "未收录域名策略：proxy（默认走代理）| resolve（本地解析查 IP）")
	reload := flag.Duration("reload", 10*time.Minute, "列表重载间隔（0 关闭）")
	flag.Parse()

	if *ipList == "" && *domainList == "" {
		fmt.Fprintln(os.Stderr, "至少需要 -ip-list 或 -domain-list 之一")
		os.Exit(2)
	}

	p := splitd.PolicyProxy
	if *policy == "resolve" {
		p = splitd.PolicyResolve
	}

	opts := splitd.Options{
		IPListPath:     *ipList,
		DomainListPath: *domainList,
		Policy:         p,
	}
	engine := splitd.New(p)
	ips, doms, err := engine.Load(opts)
	if err != nil {
		log.Fatalf("载入列表失败: %v", err)
	}
	log.Printf("已载入 %d 条 IP 段、%d 个域名（策略 %s）", ips, doms, *policy)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := splitd.Serve(ctx, *addr, engine, opts, *reload, log.Printf); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}
