package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/egoist/mygo/ui"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

func newTestApp(t *testing.T) *app {
	t.Helper()
	st, err := settings.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := profiles.LoadStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &app{
		page: "main",
		st:   st,
		ps:   ps,
		pm:   proc.NewManager(nil),
	}
}

// 视图在无窗口的 Tester 中构建，验证各页面可渲染且文本正确。

func TestMainEmpty(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("还没有配置") {
		t.Errorf("empty state should guide to add, texts %q", tt.Texts())
	}
	if !tt.HasText("添加配置") {
		t.Errorf("empty state should offer CTA, texts %q", tt.Texts())
	}
	// 空状态下不应渲染列表末尾的添加卡片（那是完整形态才有的）
	if tt.HasText("＋ 添加配置") {
		t.Errorf("empty state must not render dashboard, texts %q", tt.Texts())
	}
}

func TestMainWithProfile(t *testing.T) {
	a := newTestApp(t)
	a.binFound = true
	if _, err := a.ps.Save(&profiles.Profile{
		Name: "测试代理", Kind: profiles.KindProxy, Listen: ":8080", Proto: "http",
	}); err != nil {
		t.Fatal(err)
	}
	a.st.Update(func(s *settings.Settings) { s.Current = a.ps.Items()[0].ID })
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("测试代理") {
		t.Errorf("profile row should show name, texts %q", tt.Texts())
	}
	if !tt.HasText(":8080") {
		t.Errorf("profile row should show listen addr, texts %q", tt.Texts())
	}
	if !tt.HasText("连接") {
		t.Errorf("should offer connect, texts %q", tt.Texts())
	}
	if !tt.HasText("＋ 添加配置") {
		t.Errorf("list should end with add card, texts %q", tt.Texts())
	}
}

func TestLogsView(t *testing.T) {
	a := newTestApp(t)
	a.page = "logs"
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("暂无日志") {
		t.Errorf("logs page should render empty state, texts %q", tt.Texts())
	}
}

func TestSettingsView(t *testing.T) {
	a := newTestApp(t)
	a.page = "settings"
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("GOST 程序") {
		t.Errorf("settings should group by card, texts %q", tt.Texts())
	}
	if !tt.HasText("下载 / 更新") {
		t.Errorf("settings should offer a gost download action, texts %q", tt.Texts())
	}
	if !tt.HasText("行为") {
		t.Errorf("settings should show behavior card, texts %q", tt.Texts())
	}
}

func TestEditorView(t *testing.T) {
	a := newTestApp(t)
	a.startNew()
	tt := ui.NewTester(a.view, 430, 660)
	if !tt.HasText("保存") {
		t.Errorf("editor should render form actions, texts %q", tt.Texts())
	}
	if !tt.HasText("本机监听") {
		t.Errorf("proxy form should show the listener section, texts %q", tt.Texts())
	}
}

// 防回归：时长必须按秒计算，不得把纳秒当秒。
func TestFmtDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{5 * time.Second, "0:05"},
		{90 * time.Second, "1:30"},
		{2*time.Hour + 3*time.Minute + 4*time.Second, "2:03:04"},
	}
	for _, c := range cases {
		if got := fmtDuration(c.d); got != c.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

// 主题派生：深浅两套都可派生且主色/语义色不为零值。
func TestAppTheme(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := appTheme(dark)
		if th.Dark != dark {
			t.Errorf("appTheme(%v).Dark = %v", dark, th.Dark)
		}
		if th.Accent == (ui.Color{}) || th.Success == (ui.Color{}) || th.Danger == (ui.Color{}) {
			t.Errorf("theme colors must be set (dark=%v)", dark)
		}
		if th.Spacing != sp1 || th.Radius != rCtl {
			t.Errorf("theme must follow spacing/radius tokens (dark=%v)", dark)
		}
	}
}

// 日志行着色：错误/警告用语义色，普通行用弱化文本色。
func TestLogLineColor(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := appTheme(dark)
		if got := logLineColor(th, "12:00:01 [ERRO] dial failed"); got != th.Danger {
			t.Errorf("error line should be danger (dark=%v)", dark)
		}
		if got := logLineColor(th, "12:00:02 [WARN] retry"); got != th.Warning {
			t.Errorf("warn line should be warning (dark=%v)", dark)
		}
		if got := logLineColor(th, "12:00:03 listen on :1080"); got != th.TextMuted {
			t.Errorf("plain line should be muted (dark=%v)", dark)
		}
	}
}

// 选中/光环/提示底色在深浅两套下都必须有透明度而不透明遮住内容。
func TestSoftColors(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := appTheme(dark)
		for name, c := range map[string]ui.Color{
			"accentSoft":  accentSoft(th),
			"successHalo": successHalo(th),
			"dangerSoft":  dangerSoft(th),
			"warningSoft": warningSoft(th),
		} {
			if c == (ui.Color{}) {
				t.Errorf("%s must not be zero (dark=%v)", name, dark)
			}
		}
	}
}

// 布局回归：光环必须比按钮大一圈，且两者尺寸显式（不可依赖父容器拉伸）。
func TestConnectGeometry(t *testing.T) {
	btn, halo := connectGeometry()
	if btn <= 0 || halo != btn+sp3*2 {
		t.Fatalf("halo(%v) 应为 button(%v)+2*sp3(%v)", halo, btn, sp3)
	}
}

// 表单对齐回归：同一节的等宽两列，跨行的同列字段必须左边缘对齐。
// 曾因两行各自 Grow(1) 分配比例不同而错位。
func TestFormColumnsAligned(t *testing.T) {
	a := newTestApp(t)
	a.startNew()
	a.form.UpEnabled = true
	tt := ui.NewTester(a.view, 430, 900)

	x := func(label string) float32 {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("label %q not found", label)
		}
		return r.X
	}
	// ① 本机监听：同一列跨行必须同 x
	if got, want := x("账号（可选）"), x("地址"); got != want {
		t.Errorf("左列未对齐：地址 x=%.1f，账号 x=%.1f", want, got)
	}
	if got, want := x("密码（可选）"), x("协议"); got != want {
		t.Errorf("右列未对齐：协议 x=%.1f，密码 x=%.1f", want, got)
	}
	// ② 上游节点：同样规则
	if got, want := x("服务器账号（可选）"), x("服务器协议"); got != want {
		t.Errorf("上游左列未对齐：服务器协议 x=%.1f，服务器账号 x=%.1f", want, got)
	}
	if got, want := x("服务器密码（可选）"), x("服务器地址"); got != want {
		t.Errorf("上游右列未对齐：服务器地址 x=%.1f，服务器密码 x=%.1f", want, got)
	}
}

// 悬停提示要包含状态、配置、端口与时长。
func TestTrayTooltip(t *testing.T) {
	start := time.Now().Add(-90 * time.Second)
	got := trayTooltip(true, proc.Running, "hk", ":8027", start)
	for _, want := range []string{"运行中", "hk", ":8027", "1:30"} {
		if !strings.Contains(got, want) {
			t.Errorf("tooltip %q missing %q", got, want)
		}
	}
	if got := trayTooltip(false, proc.Stopped, "", "", time.Time{}); !strings.Contains(got, "未连接") {
		t.Errorf("stopped tooltip %q", got)
	}
	if got := trayTooltip(false, proc.Crashed, "", "", time.Time{}); !strings.Contains(got, "异常") {
		t.Errorf("crashed tooltip %q", got)
	}
}

// 托盘菜单：状态行只读、动作随状态变化、含配置子菜单与日志/设置入口。
func TestTrayMenuShape(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.ps.Save(&profiles.Profile{Name: "hk", Kind: profiles.KindProxy, Listen: ":8027", Proto: "http"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	items := a.trayMenuItems(true, proc.Running, "hk", ":8027", start)
	if !items[0].Disabled || !strings.Contains(items[0].Label, "运行中") {
		t.Errorf("first item should be a read-only status line: %+v", items[0])
	}
	labels := make([]string, 0, len(items))
	for _, it := range items {
		labels = append(labels, it.Label)
	}
	for _, want := range []string{"断开", "配置", "日志…", "设置…"} {
		found := false
		for _, l := range labels {
			if l == want {
				found = true
			}
		}
		if !found {
			t.Errorf("menu missing %q: %v", want, labels)
		}
	}

	idle := a.trayMenuItems(false, proc.Stopped, "", "", time.Time{})
	if idle[0].Label != "未连接" {
		t.Errorf("idle status line %q", idle[0].Label)
	}
	if idle[2].Label != "连接" {
		t.Errorf("idle action %q", idle[2].Label)
	}

	sub := a.trayProfileItems()
	if len(sub) < 2 || sub[0].Type != mygo.MenuItemRadio || sub[0].Label != "hk" {
		t.Errorf("config submenu: %+v", sub)
	}
}
