package splitd

import (
	"context"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/go-gost/plugin/bypass/proto"
)

// Server 实现 gost 的 bypass gRPC 插件协议。
type Server struct {
	proto.UnimplementedBypassServer
	engine *Engine
	hits   atomic.Int64
	misses atomic.Int64
}

// NewServer 创建服务。
func NewServer(engine *Engine) *Server { return &Server{engine: engine} }

// Bypass 判定一次请求：Ok=true 表示直连（跳过转发链）。
func (s *Server) Bypass(_ context.Context, in *proto.BypassRequest) (*proto.BypassReply, error) {
	target := in.GetAddr()
	if target == "" {
		// 部分客户端只给 Host
		target = in.GetHost()
	}
	bypass := s.engine.ShouldBypass(target)
	if bypass {
		s.hits.Add(1)
	} else {
		s.misses.Add(1)
	}
	return &proto.BypassReply{Ok: bypass}, nil
}

// Stats 返回直连/代理的判定次数。
func (s *Server) Stats() (hits, misses int64) {
	return s.hits.Load(), s.misses.Load()
}

// Serve 在 addr 上提供 gRPC 服务，直到 ctx 结束。
// reload 期间每隔 reloadEvery 重载列表（<=0 表示不重载）。
func Serve(ctx context.Context, addr string, engine *Engine, opts Options, reloadEvery time.Duration, logf func(string, ...any)) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if reloadEvery > 0 {
		go func() {
			t := time.NewTicker(reloadEvery)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					ips, doms, err := engine.Load(opts)
					if err != nil {
						logf("重载列表失败: %v", err)
						continue
					}
					logf("已重载列表: %d IP / %d 域名", ips, doms)
				}
			}
		}()
	}

	srv := grpc.NewServer()
	proto.RegisterBypassServer(srv, NewServer(engine))

	go func() {
		<-ctx.Done()
		srv.Stop()
	}()

	if logf != nil {
		logf("分流插件监听 %s", addr)
	}
	return srv.Serve(lis)
}
