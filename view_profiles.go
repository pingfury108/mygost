package main

import (
	"bytes"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gopkg.in/yaml.v3"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

var kindNames = []string{"代理", "端口转发", "YAML"}
var kindValues = []profiles.Kind{profiles.KindProxy, profiles.KindForward, profiles.KindCustom}

// proxyHandlers 是"代理"类型的可用协议：决定本机以什么协议对外提供代理。
var proxyHandlers = []string{"http", "socks5", "socks4", "ss", "relay", "sni"}

// viewProfileForm 新建/编辑表单：按 ①②③ 分节，与 gost 命令逐段对应。
func (a *app) viewProfileForm(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		// 顶部固定：类型切换是模式开关，滚动表单时也应常驻。
		// 用分段控件与全站控件语言统一（设置页、日志页同款）。
		ui.Column(c).FillWidth().Padding(sp2, sp4, 0).Children(func() {
			if segmentedFill(c, &a.formKind, kindNames...) {
				a.syncFormKind()
				a.formErr = ""
			}
		})

		// 中间区：表单滚动；YAML 编辑器直接占满可视区（放进滚动容器会算不出高度）
		if kindValues[a.formKind] == profiles.KindCustom {
			ui.Box(c).FillWidth().Grow(1).Padding(sp3, sp4, sp2).Children(func() {
				a.customForm(c)
			})
		} else {
			ui.Scroll(c).FillWidth().Grow(1).Shrink(1).Children(func() {
				ui.Column(c).FillWidth().Padding(sp3, sp4, sp2).Gap(sp4).Children(func() {
					if kindValues[a.formKind] == profiles.KindForward {
						a.forwardForm(c)
					} else {
						a.proxyForm(c)
					}
				})
			})
		}

		// 固定底栏：等效命令 + 错误提示 + 操作，始终可见
		ui.Column(c).FillWidth().Gap(sp2).Padding(sp2, sp4, sp4).Children(func() {
			// 等效命令：界面填写的内容翻译成命令行。
			// 用只读输入框而非文本：超宽时可横向滚动，也能选中复制。
			ui.Column(c).FillWidth().Gap(sp1).Children(func() {
				ui.Text(c, "等效命令").FontSize(fsCaption).TextColor(t.TextMuted)
				a.cmdPreview = a.commandPreview()
				ui.TextInput(c, &a.cmdPreview).ReadOnly(true).
					Font("monospace").FontSize(fsMono)
			})
			if a.formErr != "" {
				ui.Text(c, a.formErr).FontSize(fsCaption).TextColor(t.Danger)
			}
			ui.Row(c).FillWidth().Gap(sp2).Children(func() {
				cancel := ui.ButtonBase(c).Width(96).Padding(sp3+2, 0).Radius(10).
					Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorPointer).
					Center().Children(func() {
					ui.Text(c, tr("取消", "Cancel")).FontSize(fsBody).Bold().TextColor(t.TextMuted)
				})
				if cancel.Clicked() {
					a.formErr = ""
					a.goMain()
				}
				saveLabel := tr("保存", "Save")
				if kindValues[a.formKind] == profiles.KindCustom {
					saveLabel = tr("校验并保存", "Validate & Save")
				}
				ui.PrimaryButton(c, saveLabel).Grow(1).OnClick(a.saveEdit)
			})
		})
	})
}

// formSection 是表单里的一节：大标题 + 一句话说明 + 内容。
func formSection(c *ui.Context, title, desc string, body func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(sp2).Children(func() {
		ui.Column(c).FillWidth().Gap(1).Children(func() {
			ui.Text(c, title).FontSize(fsBody).Bold()
			if desc != "" {
				ui.Text(c, desc).FontSize(fsCaption).TextColor(t.TextMuted)
			}
		})
		body()
	})
}

// proxyForm 是"代理"表单：
//
//	① 本机监听  = gost -L
//	② 出网方式  = 直连 / -F 上游
//	③ 名称
func (a *app) proxyForm(c *ui.Context) {
	t := c.Theme()
	card(c).Children(func() {
		ui.Column(c).FillWidth().Gap(sp4).Children(func() {
			formSection(c, "① 本机监听", "在这台机器上开一个代理端口", func() {
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					formRow(c, tr("监听地址", "Listen"), tr("留空则自动选择端口", "Empty to auto-pick"), func() {
						ui.TextInput(c, &a.form.Listen).Width(formCtlW).
							Font("monospace").Placeholder(":8080")
					})
					formRow(c, tr("协议", "Protocol"), tr("本机以什么协议对外提供代理", "Local proxy protocol"), func() {
						// 常用的 http/socks5 用分段（设计稿 A5）；冷门协议回退下拉
						if a.form.Proto == "http" || a.form.Proto == "socks5" || a.form.Proto == "" {
							a.protoSeg = b2i(a.form.Proto == "socks5")
							if segmentedFixed(c, &a.protoSeg, formCtlW, "http", "socks5") {
								a.form.Proto = []string{"http", "socks5"}[a.protoSeg]
							}
						} else {
							ui.Select(c, &a.form.Proto, proxyHandlers).Width(formCtlW)
						}
					})
					formRow(c, tr("本机认证", "Auth"), tr("可选 · 留空无需认证", "Optional"), func() {
						ui.Row(c).Gap(6).Children(func() {
							half := (formCtlW - 6) / 2
							ui.TextInput(c, &a.form.User).Width(half).Placeholder(tr("账号", "user"))
							ui.TextInput(c, &a.form.Password).Width(half).Password().Placeholder(tr("密码", "····"))
						})
					})
				})
			})

			divider(c)

			formSection(c, "② 出网方式", "流量从哪里连到目标", func() {
				// 分段切换比 radio 更明确：直连 / 经上游
				a.upSeg = b2i(a.form.UpEnabled)
				if segmentedFill(c, &a.upSeg, tr("直连目标", "Direct"),
					tr("经上游服务器", "Via upstream")) {
					a.form.UpEnabled = a.upSeg == 1
				}
				if a.form.UpEnabled {
					// 嵌套上游卡：淡底色区分层级；控件自解释（下拉即协议，placeholder 即账号密码）。
					// 两行的首列必须同宽，否则中间的竖向分界错开，看起来很乱。
					ui.Box(c).FillWidth().Padding(sp2).Radius(rRow).
						Background(t.Background).Children(func() {
						ui.Column(c).FillWidth().Gap(6).Children(func() {
							ui.Row(c).FillWidth().Gap(6).Children(func() {
								// MinWidth(0)：Select 自带内容最小宽（箭头+文字），不压就会比账号框宽
								ui.Select(c, &a.form.UpProto, profiles.ConnectorTypes).
									Width(upCtlW).MinWidth(0)
								ui.TextInput(c, &a.form.UpAddr).Grow(1).
									Font("monospace").Placeholder(tr("主机:端口", "host:port"))
							})
							ui.Row(c).FillWidth().Gap(6).Children(func() {
								ui.TextInput(c, &a.form.UpUser).Width(upCtlW).MinWidth(0).
									Placeholder(tr("账号（可选）", "user (optional)"))
								ui.TextInput(c, &a.form.UpPass).Grow(1).Password().
									Placeholder(tr("密码（可选）", "password (optional)"))
							})
						})
					})
				}
			})

			divider(c)

			formSection(c, "③ 名称", "只用于列表显示", func() {
				formRow(c, tr("名称", "Name"), "", func() {
					ui.TextInput(c, &a.form.Name).Grow(1).Placeholder("如：家里出口")
				})
			})
		})
	})
}

// forwardForm 是"端口转发"表单：
//
//	① 本机监听 -L ② 转发目标 ③ 名称
func (a *app) forwardForm(c *ui.Context) {
	t := c.Theme()
	card(c).Children(func() {
		ui.Column(c).FillWidth().Gap(sp4).Children(func() {
			formSection(c, "① 本机监听", "在这台机器上开一个端口", func() {
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					formRow(c, tr("监听地址", "Listen"), tr("留空则自动选择端口", "Empty to auto-pick"), func() {
						ui.TextInput(c, &a.form.Listen).Width(formCtlW).
							Font("monospace").Placeholder(":2222")
					})
					formRow(c, tr("协议", "Protocol"), tr("tcp 适用于 SSH、网页等服务", "tcp for SSH, web, etc."), func() {
						fwdSeg := 0
						if a.form.FwdProto == "udp" {
							fwdSeg = 1
						}
						if segmentedFixed(c, &fwdSeg, formCtlW, "tcp", "udp") {
							a.form.FwdProto = []string{"tcp", "udp"}[fwdSeg]
						}
					})
				})
			})

			divider(c)

			formSection(c, "② 转发到", "连接本机端口时，实际连到这些地址（多个即负载均衡）", func() {
				// 行式目标列表：比多行文本框更直观，多目标负载均衡看得见
				a.syncTargets() // 兑底：保证等效命令与最新行一致
				if len(a.fwdTargets) == 0 {
					a.fwdTargets = []string{""}
				}
				ui.Column(c).FillWidth().Gap(sp2).Children(func() {
					for i := range a.fwdTargets {
						idx := i // 闭包捕获
						ui.Row(c).FillWidth().Gap(sp2).AlignItems(ui.Center).Children(func() {
							ui.TextInput(c, &a.fwdTargets[idx]).Grow(1).
								Font("monospace").Placeholder("192.168.1.10:22").
								OnChange(a.syncTargets)
							del := iconButton(c, "✕")
							if del.Clicked() && len(a.fwdTargets) > 1 {
								a.fwdTargets = append(a.fwdTargets[:idx], a.fwdTargets[idx+1:]...)
								a.syncTargets()
							}
						})
					}
					add := ui.ButtonBase(c).FillWidth().Height(32).Radius(rRow).
						Background(c.Theme().SurfaceHover).Cursor(ui.CursorPointer).
						Children(func() {
							ui.Text(c, "＋ "+tr("添加目标", "Add target")).FontSize(fsCaption).
								Bold().TextColor(c.Theme().Accent)
						})
					if add.Clicked() {
						a.fwdTargets = append(a.fwdTargets, "")
					}
				})
				// 多目标语义：设计稿 A7 的蓝点提示行
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).Children(func() {
					statusDot(c, c.Theme().Accent, 7)
					ui.Text(c, tr("多个目标自动负载均衡 · 按顺序依次尝试",
						"Multiple targets are load-balanced in order")).
						FontSize(fsCaption).TextColor(t.TextMuted)
				})
			})

			divider(c)

			formSection(c, "③ 名称", "只用于列表显示", func() {
				ui.TextInput(c, &a.form.Name).Placeholder("如：家里 NAS")
			})
		})
	})
}

// syncTargets 把目标行写回 form.Targets（等效命令与保存都用它），空行自动忽略。
func (a *app) syncTargets() {
	kept := make([]string, 0, len(a.fwdTargets))
	for _, s := range a.fwdTargets {
		if v := strings.TrimSpace(s); v != "" {
			kept = append(kept, v)
		}
	}
	a.form.Targets = strings.Join(kept, "\n")
}

func (a *app) customForm(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(sp2).Children(func() {
		ui.Text(c, "直接编辑 gost.yml，支持 gost 全部能力。").
			FontSize(fsCaption).TextColor(t.TextMuted)
		// 编辑器占满剩余高度；缓冲缺失时兜底创建，避免出现空白区
		ui.Box(c).FillWidth().Grow(1).Radius(rCard).Clip().
			Background(t.Background).Border(1, t.Border).Children(func() {
			if a.customBuf == nil {
				a.customBuf = newYAMLBuffer(a.yamlSeed())
			}
			editor.View(c, a.customBuf).Fill()
		})
		// 实时校验：语法是否正确一眼可见，不用等保存
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(sp2).Children(func() {
			if yamlValid(a.customBuf.Buffer.String()) {
				statusDot(c, t.Success, 7)
				ui.Text(c, tr("YAML 校验通过", "YAML is valid")).FontSize(fsCaption).TextColor(t.TextMuted)
			} else {
				statusDot(c, t.Danger, 7)
				ui.Text(c, tr("YAML 语法有误 · 检查缩进与冒号", "Invalid YAML · check indent and colons")).
					FontSize(fsCaption).TextColor(t.Danger)
			}
		})
	})
}

// commandPreview 把当前表单翻译成 gost 命令行，供用户与文档对照。
func (a *app) commandPreview() string {
	listen := strings.TrimSpace(a.form.Listen)
	if listen == "" {
		switch kindValues[a.formKind] {
		case profiles.KindForward:
			listen = ":2222"
		default:
			listen = ":8080"
		}
	}
	proto := a.form.Proto
	switch kindValues[a.formKind] {
	case profiles.KindForward:
		proto = orDefault(a.form.FwdProto, "tcp")
		first := firstTarget(a.form.Targets)
		if first == "" {
			first = "目标地址"
		}
		return "gost -L=" + proto + "://" + listen + "/" + first
	case profiles.KindCustom:
		return "gost -C gost.yml"
	}

	proto = orDefault(proto, "http")
	cmd := "gost -L=" + proto + "://"
	if a.form.User != "" {
		cmd += a.form.User + ":***@"
	}
	cmd += listen
	if a.form.UpEnabled {
		cmd += " " + a.chainPreview()
	}
	return cmd
}

// chainPreview 返回上游节点的 gost 等价写法（-F ...），密码打码。
func (a *app) chainPreview() string {
	proto := orDefault(a.form.UpProto, "socks5")
	auth := ""
	if a.form.UpUser != "" {
		auth = a.form.UpUser
		if a.form.UpPass != "" {
			auth += ":***"
		}
		auth += "@"
	}
	return "-F " + proto + "://" + auth + a.form.UpAddr
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func firstTarget(targets string) string {
	for _, line := range strings.FieldsFunc(targets, func(r rune) bool { return r == '\n' || r == ',' }) {
		if v := strings.TrimSpace(line); v != "" {
			return v
		}
	}
	return ""
}

// formName 名称字段（保留给 YAML 之外的简化调用）。
func formName(c *ui.Context, a *app) {
	ui.Field(c, "名称", func() {
		ui.TextInput(c, &a.form.Name).Placeholder("如：本机代理")
	})
}

// startNew 进入新建表单。
func (a *app) startNew() {
	a.form = profiles.Profile{
		Kind: profiles.KindProxy, Proto: "http",
		FwdProto: "tcp", UpProto: "socks5",
	}
	a.formKind = 0
	a.formUpOpen = true
	a.formErr = ""
	a.customBuf = nil
	a.upSeg = 0
	a.fwdTargets = []string{""}
	a.page = "editor"
}

// startEdit 进入编辑表单。
func (a *app) startEdit(p *profiles.Profile) {
	a.form = *p
	a.formKind = 0
	for i, k := range kindValues {
		if k == p.Kind {
			a.formKind = i
		}
	}
	a.formUpOpen = p.UpEnabled
	a.formErr = ""
	a.customBuf = nil
	a.upSeg = b2i(p.UpEnabled)
	a.fwdTargets = splitTargets(p.Targets)
	if p.Kind == profiles.KindCustom {
		a.customBuf = newYAMLBuffer(p.YAML)
	}
	a.page = "editor"
}

func (a *app) formTitle() string {
	if a.form.ID == "" {
		return "新建配置"
	}
	return "编辑配置"
}

// syncFormKind 在切换类型时准备 custom 编辑器缓冲。
func (a *app) syncFormKind() {
	if kindValues[a.formKind] == profiles.KindCustom {
		a.customBuf = newYAMLBuffer(a.yamlSeed())
	}
}

// yamlTemplate 是 YAML 模式的起始内容：有表单内容就沿用，否则给一个最小可用配置。
const yamlTemplate = `services:
  - name: mygost
    addr: ":8080"
    handler:
      type: http
    listener:
      type: tcp
`

// yamlSeed 返回 YAML 编辑器的初始文本。
func (a *app) yamlSeed() string {
	if src := strings.TrimSpace(a.form.YAML); src != "" {
		return src
	}
	// 表单能生成就用表单生成的，用户能看到自己刚填的内容。
	// 用副本生成，避免在渲染过程里改动应用状态。
	cp := a.form
	cp.Kind = profiles.KindProxy
	if y, err := cp.BuildYAML(); err == nil && len(bytes.TrimSpace(y)) > 0 {
		return string(y)
	}
	return yamlTemplate
}

// saveEdit 校验并保存。
func (a *app) saveEdit() {
	a.form.Kind = kindValues[a.formKind]
	if a.form.Kind == profiles.KindForward {
		a.syncTargets()
	}
	if a.form.Kind == profiles.KindCustom && a.customBuf != nil {
		a.form.YAML = a.customBuf.Buffer.String()
		if !yamlValid(a.form.YAML) {
			a.formErr = "YAML 语法有误，请检查缩进与冒号"
			return
		}
	}
	if strings.TrimSpace(a.form.Name) == "" {
		a.formErr = "请填写名称"
		return
	}
	if _, err := a.ps.Save(&a.form); err != nil {
		a.formErr = err.Error()
		return
	}
	a.formErr = ""
	a.update(func(s *settings.Settings) {
		if s.Current == "" {
			s.Current = a.form.ID
		}
	})
	a.page = "main"
	a.toast("已保存", "success")
}

// newYAMLBuffer 建一个启用 YAML 高亮的编辑缓冲。
func newYAMLBuffer(src string) *editor.State {
	st := &editor.State{Buffer: editor.NewBuffer(src), SoftWrap: true}
	st.Buffer.SetLanguage(editor.LanguageFor("gost.yml"))
	return st
}

// yamlValid 校验 YAML 语法。
func yamlValid(s string) bool {
	var v map[string]any
	return yaml.Unmarshal([]byte(s), &v) == nil
}

// b2i bool 转分段索引。
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// splitTargets 把 Targets 拆成行（至少保留一行供编辑）。
func splitTargets(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, strings.TrimSpace(line))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}
