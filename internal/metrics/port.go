package metrics

import (
	"fmt"
	"net"
)

// PickPort 从 start 起找一个可监听的端口，用于 gost 的 metrics 端点。
// 最多尝试 tries 个；全部占用时返回错误。
func PickPort(start, tries int) (int, error) {
	if start <= 0 {
		start = 9100
	}
	if tries <= 0 {
		tries = 10
	}
	for p := start; p < start+tries; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		ln.Close()
		return p, nil
	}
	return 0, fmt.Errorf("9100 起 %d 个端口都被占用", tries)
}
