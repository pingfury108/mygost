package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	// MyGo 自身文案（菜单 Edit/View/Window、Undo/Copy 等）的中文翻译，
	// 未 import 时框架字符串永远是英文，导致菜单中英混排。
	_ "github.com/egoist/mygo/locales/zh-Hans"
	_ "github.com/egoist/mygo/locales/zh-Hant"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"mygost/internal/geo"
	"mygost/internal/gostbin"
	"mygost/internal/metrics"
	"mygost/internal/probe"
	"mygost/internal/proc"
	"mygost/internal/profiles"
	"mygost/internal/reqlog"
	"mygost/internal/settings"
)

// app 是应用状态；视图是它的函数。字段只在主线程（或经 Window.Update）读写。
type app struct {
	win  *mygo.Window
	st   *settings.Store
	ps   *profiles.Store
	loc  *gostbin.Locator
	pm   *proc.Manager
	geoM *geo.Manager
	mc   *metrics.Client
	msp  *metrics.Speed
	tray *mygo.Tray

	trayIconRunning bool
	trayIconSet     bool
	trayStateMu     sync.Mutex
	trayState       proc.State
	trayPID         int
	trayStateSet    bool

	// 残留进程扫描缓存：pgrep 要遍历整张进程表，不能每 2 秒跑一次。
	leftoverMu   sync.Mutex
	leftoverAt   time.Time
	leftoverPids []int
	leftoverOK   bool

	// 页面导航（窄窗口：主界面 + 推入式子页）
	page string // main / logs / settings / editor

	// 主界面
	binPath   atomic.Value
	binSource string
	binVer    string
	binFound  bool
	binErr    string // 查找/解压 gost 失败的原因
	dlBusy    bool
	dlDone    int64
	dlTotal   int64

	// 配置档编辑
	editing    bool
	form       profiles.Profile
	formKind   int    // Segmented 索引
	formUpOpen bool   // 上游转发链折叠区
	formErr    string // 表单内联错误
	cmdPreview string // 等效命令（只读，每帧重算）
	customBuf  *editor.State
	yamlCheck  *editor.State // 上次校验的缓冲（按版本复用结果）
	yamlVer    uint64
	yamlOK     bool
	upSeg      int      // 代理表单：出网分段 0=直连 1=经上游
	protoSeg   int      // 代理表单：协议分段 0=http 1=socks5
	fwdTargets []string // 端口转发表单：目标行（每行一个地址）

	// 日志页
	logView       int    // 0=原始 1=请求
	reqDay        string // 请求视图看哪一天（空=实时，否则 YYYY-MM-DD）
	reqOnlyFailed bool   // 请求视图：只看失败
	reqs          *reqlog.Log

	logFilter string
	logFollow bool // 自动跟随最新日志（可关闭）
	logSeq    uint64

	// 请求历史视图缓存：历史文件写完即不变，不能每帧重读整个 JSONL。
	reqDayEntries []reqlog.Entry
	reqDayLoaded  string
	reqDayLoading string
	reqDayCount   int

	// reqInvalidateAt 是请求视图上次重绘时间（UnixNano），高频日志下合并重绘。
	reqInvalidateAt     atomic.Int64
	reqInvalidateQueued atomic.Bool

	// 主题安装状态（跟随系统深浅色）
	themeApplied bool
	themeDark    bool
	// quitting 置位后放行窗口关闭：否则 OnClose 的 PreventDefault 会卡死 Quit 流程
	quitting bool

	// 设置页保存反馈（短时提示）
	savedUntil time.Time

	// 连接测试与流量统计
	testing     bool                      // 正在测试连通性
	testResByID map[string][]probe.Result // 各配置的测试结果（内联显示）
	metricsOK   bool                      // 是否成功抓到数据
	rates       metrics.Rates             // 最近一次速率
	metricsPort atomic.Int32              // gost metrics 端口（0=未启用）
	metricsGen  atomic.Int64              // 采样器代际，重启时淘汰旧循环
	stats       *metrics.Store            // 按天流量统计
	statsSaved  time.Time                 // 上次落盘时间
	sessBase    metrics.Snapshot          // 会话基准（累加增量的起点）
	hasSessBase bool

	// runningProfile 是实际启动/接管的配置 ID。当前选中项可以随后改变，
	// 但"正在运行的是谁"不能跟着变。
	runningProfile atomic.Value

	// 待展示的 toast（视图构建时 flush 到窗口）
	pendingToasts []ui.Toast

	// 删除确认
	delOpen   bool
	delTitle  string
	delTarget string

	// 设置页表单
	sGostPath    string
	sMirror      string
	sKeep        bool
	sAuto        bool
	sLogin       bool
	sSplit       bool
	sSplitURL    string
	sSplitPolicy int // 0=未收录域名走代理 1=按 IP 判断
	sReqLog      bool
	splitBusy    bool // 分流列表更新中

	saveTimer *time.Timer // 文本输入防抖，停顿后才落盘

	// splitSup 是分流插件的唯一管理者（启停只走它，UI 不碰）
	splitSup *splitSupervisor

	// opMu/opBusy 串行化启动与停止：自动启动与用户点击可能并发
	opMu   sync.Mutex
	opBusy bool

	// dataDirSrc 说明数据目录来自哪里（系统默认 / 自定义 / 环境变量）
	dataDirSrc string
	// dataDirWarn 是回退到默认目录的原因（自定义目录不可用时非空）
	dataDirWarn string
	// pendingDataDir 是待确认的新目录（用户刚选完，还没落盘）
	pendingDataDir string
	// dataDirRestart 表示目录已改，等待重启生效
	dataDirRestart bool

	// logPath 是文件日志路径（设置页可打开，Windows 上排障主要靠它）
	logPath string

	// vm 是视图需要的派生状态，只由后台探测写（refreshViewModel），
	// view 只读。渲染路径里不能有 exec/dial/读大文件，否则滚动会卡。
	vm viewModel
}

// viewModel 是界面的只读快照：重操作的结果全部落在这里。
type viewModel struct {
	splitAlive  bool      // 分流插件服务可用（含复用的外部实例）
	splitAddr   string    // 插件实际监听地址（8100 被占时会变）
	splitErr    string    // 最近一次插件启动失败原因
	leftovers   []int     // 不被本会话管理的 gost 进程
	domainCount int       // 域名规则数
	ipCount     int       // IP 规则数
	rulesReady  bool      // 两份规则文件均已下载
	rulesAt     time.Time // 规则文件更新时间（取较新的一份）
	ipAt        time.Time // IP 列表更新时间
	domainAt    time.Time // 域名列表更新时间
	reqDays     []string  // 可回看的请求记录日期
	reqToday    int       // 今日已写记录数
	reqLimit    int       // 单日记录上限
}

var dataDir string

// dataDirName 返回数据目录名：开发版加 -dev 后缀，与正式版完全隔离。
//
// 两边各自持有自己的 gost 二进制、分流规则、PID 文件与配置，
// 所以 dev 里断开/退出/清理残留都只会影响 dev 自己。
func dataDirName() string {
	if mygo.IsDev() {
		return "mygost-dev"
	}
	return "mygost"
}

// resolveDataDir 确定数据目录，返回 (目录, 来源说明, 回退原因)。
//
// 优先级：
//
//  1. MYGOST_DATA_DIR 环境变量（脚本/运维用）
//  2. 默认目录下的指针文件（用户在设置里改过目录）
//  3. 默认目录：Windows %APPDATA%\mygost，macOS ~/Library/Application Support/mygost
//
// 指针指向的目录不可用时回退默认目录，并把原因带回界面——不阻塞启动。
func resolveDataDir() (dir, src, warn string) {
	if v := cleanDirEnv(os.Getenv("MYGOST_DATA_DIR")); v != "" {
		return v, "环境变量 MYGOST_DATA_DIR", ""
	}
	def := defaultDataDir()
	if p := readPointer(); p != "" {
		if err := ensureWritableDir(p); err == nil {
			return p, "自定义目录", ""
		} else {
			return def, "系统默认目录",
				fmt.Sprintf("自定义数据目录不可用，已回退默认位置：%v", err)
		}
	}
	return def, "系统默认目录", ""
}

// cleanDirEnv 清理目录类环境变量：去空白、去包裹引号、去末尾分隔符。
//
// Windows 用户在系统属性里填值时经常连引号一起粘进来，
// 或者写成 D:\dir\ —— 直接拿去 Join 会得到奇怪的路径。
func cleanDirEnv(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = strings.TrimSpace(v[1 : len(v)-1])
		}
	}
	return v
}

// isNetworkPath 报告路径是否位于网络位置（UNC）。
// Windows 上从 UNC 路径直接执行 exe 常被安全策略拦住。
func isNetworkPath(p string) bool {
	return len(p) >= 2 && p[0] == '\\' && p[1] == '\\'
}

func main() {
	// 最早的日志：GUI 子系统下没有控制台，双击启动失败时这里是唯一线索。
	// 必须在 mygo 初始化之前就落下，否则崩溃/卡在框架里就什么都看不到。
	bootDir, _, bootWarn := resolveDataDir()
	bootLog, closeBootLog := setupLogging(bootDir)
	defer closeBootLog()
	if exe, err := os.Executable(); err == nil {
		log.Printf("启动：exe=%s", exe)
	}
	if wd, err := os.Getwd(); err == nil {
		log.Printf("      cwd=%s", wd)
	}
	log.Printf("      args=%v", os.Args)
	if bootWarn != "" {
		log.Printf("      %s", bootWarn)
	}

	if !mygo.App.RequestSingleInstanceLock() {
		log.Printf("已有实例在运行：唤起它的窗口后退出")
		return
	}
	log.Printf("单实例锁已获取")

	// 应用以中英两种语言书写：框架内建字符串（菜单角色名等）跟随系统语言
	mygo.App.SetLanguages("en", "zh-Hans")
	a := newApp()
	// GUI 子系统下 stderr 无效，日志必须落文件，否则双击出问题时无从排查
	// 让 mygo 自己的用户数据（window-state.json 等）也落在我们选的目录，
	// 否则自定义数据目录后它仍会写到系统默认位置
	mygo.App.SetPath(mygo.PathUserData, dataDir)
	a.logPath = bootLog
	// 网络位置（UNC）上直接执行解压出的 gost.exe 常被系统拦住，
	// 提前说清楚比事后让用户猜要好
	if isNetworkPath(dataDir) {
		log.Printf("警告：数据目录位于网络位置（%s），gost 可能无法执行；"+
			"可设置 MYGOST_DATA_DIR 指向本地磁盘", dataDir)
	}
	if len(dataDir) > 180 {
		log.Printf("警告：数据目录路径过长（%d 字符），Windows 下可能触发 260 字符上限", len(dataDir))
	}
	// 已经有一个实例在跑时（比如窗口被收起后再次启动图标），
	// 把那个实例的窗口叫出来，而不是静默退出让人以为没反应。
	mygo.App.OnSecondInstance(func([]string, string) { a.showMain() })

	mygo.App.WhenReady(func() {
		// 菜单/托盘文案跟随系统语言；打一行日志便于排查
		loc, zh := currentLocale()
		log.Printf("locale=%q zh=%v", loc, zh)
		mygo.App.OnLocaleChanged(func(locale string) {
			detectLang()
			a.setupAppMenu()
			a.refreshTray()
		})
		// 设置页表单初值
		s := a.st.Get()
		a.sGostPath = s.GostPath
		a.sMirror = s.Mirror
		a.sKeep = s.KeepOnExit
		a.sAuto = s.AutoStart
		a.sLogin = mygo.App.OpenAtLogin()
		// 自启动自愈：应用被移动到新路径后，注册表/LaunchAgents 里仍是旧路径，
		// 开机自启会静默失效。开着就按当前 exe 路径重写一次。
		if a.sLogin {
			if err := mygo.App.SetOpenAtLogin(true); err != nil {
				log.Printf("修正自启动项失败: %v", err)
			}
		}
		a.sSplit = s.Split
		a.sSplitURL = s.SplitIPListURL
		if s.SplitPolicy == "resolve" {
			a.sSplitPolicy = 1
		}
		a.sReqLog = s.RequestLog
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "mygost",
			Width:     430,
			Height:    660,
			MinWidth:  380,
			MinHeight: 520,
			Content:   ui.View(a.view),
		})
		log.Printf("窗口已创建：%dx%d", 430, 660)
		// 关窗口只是收起：应用继续待在菜单栏，gost 也继续运行。
		// 真正退出走菜单栏「退出 mygost」（⌘Q）。
		// Quit 流程会关闭所有窗口，此时必须放行，否则退出被卡死
		// （参见 mygo examples/menubar-native 的同名处理）。
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.quitting {
				return
			}
			e.PreventDefault()
			a.win.Hide()
		})
		// 点 Dock 图标时把窗口叫回来（macOS 无可见窗口时）
		mygo.App.OnActivate(func(hasVisibleWindows bool) {
			if !hasVisibleWindows {
				a.showMain()
			}
		})

		a.setupTray()
		a.refreshTray()
		a.setupAppMenu()
		// 分流插件由监管器统一收敛：接管了在跑的 gost 就自动补启/复用插件
		a.splitSup = newSplitSupervisor(a)
		go a.splitSup.Reconcile()
		a.splitSup.Watch()
		// 请求记录定期落盘（解析在跟，不能靠退出时才写）
		a.watchRequests()
		// 先同步探一次，首帧就有真实状态；之后每 2 秒后台刷新
		a.refreshViewModel()
		a.watchViewModel()
		a.checkBinAsync()
		if s.AutoStart && s.Current != "" {
			go a.doStart()
		}
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		a.quitting = true // 放行 OnClose 的窗口隐藏拦截
		a.saveSettings()  // 防抖计时器可能还没触发，退出前补一次
		a.flushRequests() // 请求记录落盘并关闭
		a.stopMetrics()   // 停采样并保存最后一次流量统计
		if a.stats != nil {
			if err := a.stats.Save(); err != nil {
				log.Printf("退出时保存流量统计失败: %v", err)
			}
		}
		// 插件跟随 gost：gost 保留时插件也必须保留，
		// 否则 gost 的 split-geo 会指向一个无人监听的端口，
		// 白名单模式下降级为"全部走代理"。
		if !a.st.Get().KeepOnExit {
			if err := a.pm.StopAndWait(3 * time.Second); err != nil {
				log.Printf("退出时停止 gost 失败: %v", err)
			}
			if a.splitSup != nil {
				a.splitSup.StopIfUnwanted()
			} else {
				a.geoM.Stop()
			}
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// newApp 初始化存储与进程管理器。
func newApp() *app {
	// 开发版与正式版用不同数据目录：两者各自的 gost/插件/配置互不干扰，
	// 否则 dev 启动的实例会接管（或误杀）正式版在跑的进程。
	// 必须写入包级 dataDir：main 里的 SetPath/日志/状态目录等都用它。
	var dirSrc, dirWarn string
	dataDir, dirSrc, dirWarn = resolveDataDir()
	if dirWarn != "" {
		log.Printf("%s", dirWarn)
	}

	st, err := settings.Load(dataDir)
	if err != nil {
		log.Printf("读取设置失败: %v", err)
	}
	ps, err := profiles.LoadStore(filepath.Join(dataDir, "profiles"))
	if err != nil {
		log.Printf("读取配置档失败: %v", err)
	}
	a := &app{
		page:        "main",
		dataDirSrc:  dirSrc,
		dataDirWarn: dirWarn,
		logFollow:   true,
		st:          st,
		ps:          ps,
		loc:         &gostbin.Locator{Dir: filepath.Join(dataDir, "bin")},
		geoM:        geo.NewManager(filepath.Join(dataDir, "geo"), geo.DefaultListenPort),
		mc:          metrics.NewClient(""),
		msp:         metrics.NewSpeed(5),
	}
	// 持久化的分流策略必须在插件启动前生效，否则重启后退回默认“走代理”。
	if st.Get().SplitPolicy == "resolve" {
		a.geoM.SetPolicy(geo.PolicyResolve)
	}
	// 5000 条配合 500ms 落盘，可覆盖约 1 万请求/秒的突发，
	// 不会在写盘前被环形缓冲覆盖；界面仍只展示最近部分。
	a.reqs = reqlog.New(5000)
	// 只有开启“记录请求明细”时才解析 debug 日志（gost 侧也才会输出）。
	a.reqs.SetEnabled(st.Get().RequestLog)
	a.initRequestStore(filepath.Join(dataDir, "requests"))
	a.pm = proc.NewManager(a.onProcEvent)
	a.pm.SetOnLine(a.feedRequest)
	statsStore, err := metrics.LoadStore(dataDir)
	if err != nil {
		log.Printf("读取流量统计失败: %v", err)
	}
	a.stats = statsStore
	return a
}

// onProcEvent 在后台 goroutine 被调用：日志行或进程状态变化后刷新界面。
//
// 日志行很密（debug 下每请求约 11 行），托盘菜单却只在进程状态变化时
// 需要重建；否则连跑几小时会一直在主线程造原生菜单。
func (a *app) onProcEvent() {
	if a.win == nil {
		return
	}
	st, _, _ := a.pm.State()
	pid := a.pm.Pid()
	a.trayStateMu.Lock()
	changed := !a.trayStateSet || st != a.trayState || pid != a.trayPID
	a.trayState, a.trayPID, a.trayStateSet = st, pid, true
	a.trayStateMu.Unlock()
	if changed {
		a.win.Update(func() { a.refreshTray() })
		return
	}
	a.win.Invalidate()
}

// runningLabel 返回运行状态文案：接管进程额外标注。
func (a *app) runningLabel(status string) string {
	if a.pm.ExternalPID() > 0 {
		return status + tr("（已接管）", " (attached)")
	}
	return status
}

// update 更新设置并落盘。
func (a *app) update(fn func(*settings.Settings)) {
	if err := a.st.Update(fn); err != nil {
		log.Printf("保存设置失败: %v", err)
	}
}

// toast 在窗口上弹出提示（视图构建时展示）。
func (a *app) toast(title, typ string) {
	if a.win == nil {
		return
	}
	a.win.Update(func() {
		a.pendingToasts = append(a.pendingToasts, ui.Toast{Title: title, Type: typ})
	})
}

// applyUI 把状态变更切到主线程；无窗口（测试）时直接执行。
func (a *app) applyUI(fn func()) {
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	fn()
}

// appVersion 是应用版本，与 mygo.json 保持一致（发布时同步修改）。
const appVersion = "0.2.4"

// beginOp 尝试占用启停操作槽；已有操作在进行时返回 false。
func (a *app) beginOp() bool {
	a.opMu.Lock()
	defer a.opMu.Unlock()
	if a.opBusy {
		return false
	}
	a.opBusy = true
	return true
}

// endOp 释放启停操作槽。
func (a *app) endOp() {
	a.opMu.Lock()
	a.opBusy = false
	a.opMu.Unlock()
}
