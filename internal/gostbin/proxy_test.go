package gostbin

import (
	"os"
	"strings"
	"testing"
)

// 环境变量代理应被优先读取（开发机常用）。
func TestSystemProxyFromEnv(t *testing.T) {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:8026")
	if got := systemProxy(); got != "http://127.0.0.1:8026" {
		t.Errorf("systemProxy() = %q", got)
	}
}

// 没有代理设置时返回空（表示直连）。
func TestSystemProxyEmpty(t *testing.T) {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	if got := systemProxy(); got != "" {
		t.Logf("systemProxy() = %q（平台自带代理设置，合法）", got)
	}
}

// 错误提示要给出可行动的建议，而不是只抛原始错误。
func TestExplainErrorActionable(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errString("dial tcp: i/o timeout"), "镜像"},
		{errString("no such host"), "域名解析"},
		{errString("connection refused"), "代理端口"},
		{errString("sha256 校验失败"), "镜像"},
	}
	for _, c := range cases {
		got := ExplainError(c.err, "")
		if !strings.Contains(got, c.want) {
			t.Errorf("ExplainError(%v) = %q, 应包含 %q", c.err, got, c.want)
		}
	}
}

// nil 错误返回空串。
func TestExplainErrorNil(t *testing.T) {
	if got := ExplainError(nil, ""); got != "" {
		t.Errorf("ExplainError(nil) = %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

var _ = os.Getenv
