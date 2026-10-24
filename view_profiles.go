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
		// 用文字标签页而非分段控件：无灰色轨道，视觉更轻，语义是"切换下方内容"。
		ui.Column(c).FillWidth().Padding(sp2, sp4, 0).Gap(sp2).Children(func() {
			if ui.Tabs(c, &a.formKind, kindNames...).Label("配置类型").Changed() {
				a.syncFormKind()
				a.formErr = ""
			}
			ui.Box(c).FillWidth().Height(1).Background(t.Border).Opacity(0.6)
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
				ui.Button(c, "取消").Grow(1).OnClick(func() {
					a.formErr = ""
					a.goMain()
				})
				ui.PrimaryButton(c, "保存").Grow(2).OnClick(a.saveEdit)
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
	card(c).Children(func() {
		ui.Column(c).FillWidth().Gap(sp4).Children(func() {
			formSection(c, "① 本机监听", "在这台机器上开一个代理端口", func() {
				// 等宽两列网格：跨行字段始终对齐
				ui.Grid(c).FillWidth().Columns(2).Gap(sp3).Children(func() {
					ui.Field(c, "地址", func() {
						ui.TextInput(c, &a.form.Listen).FillWidth().Placeholder(":8080")
					})
					ui.Field(c, "协议", func() {
						ui.Select(c, &a.form.Proto, proxyHandlers).FillWidth()
					})
					ui.Field(c, "账号（可选）", func() {
						ui.TextInput(c, &a.form.User).FillWidth().
							Placeholder("留空则无需认证")
					})
					ui.Field(c, "密码（可选）", func() {
						ui.TextInput(c, &a.form.Password).FillWidth().Password()
					})
				})
			})

			divider(c)

			formSection(c, "② 出网方式", "流量从哪里连到目标", func() {
				usingUp := a.form.UpEnabled
				ui.RadioGroup(c, func() {
					ui.Radio(c, &usingUp, false, "直连目标：本机直接连出去")
					ui.Radio(c, &usingUp, true, "经上游服务器：先连服务器，由它代连（-F）")
				})
				if usingUp != a.form.UpEnabled {
					a.form.UpEnabled = usingUp
				}
				if a.form.UpEnabled {
					ui.Box(c).FillWidth().Padding(sp3).Radius(rRow).
						Background(c.Theme().Background).Children(func() {
						ui.Grid(c).FillWidth().Columns(2).Gap(sp3).Children(func() {
							ui.Field(c, "服务器协议", func() {
								ui.Select(c, &a.form.UpProto, profiles.ConnectorTypes).FillWidth()
							})
							ui.Field(c, "服务器地址", func() {
								ui.TextInput(c, &a.form.UpAddr).FillWidth().
									Placeholder("主机:端口")
							})
							ui.Field(c, "服务器账号（可选）", func() {
								ui.TextInput(c, &a.form.UpUser).FillWidth()
							})
							ui.Field(c, "服务器密码（可选）", func() {
								ui.TextInput(c, &a.form.UpPass).FillWidth().Password()
							})
						})
					})
				}
			})

			divider(c)

			formSection(c, "③ 名称", "只用于列表显示", func() {
				ui.TextInput(c, &a.form.Name).Placeholder("如：家里出口")
			})
		})
	})
}

// forwardForm 是"端口转发"表单：
//
//	① 本机监听 -L ② 转发目标 ③ 名称
func (a *app) forwardForm(c *ui.Context) {
	card(c).Children(func() {
		ui.Column(c).FillWidth().Gap(sp4).Children(func() {
			formSection(c, "① 本机监听", "在这台机器上开一个端口", func() {
				ui.Grid(c).FillWidth().Columns(2).Gap(sp3).Children(func() {
					ui.Field(c, "地址", func() {
						ui.TextInput(c, &a.form.Listen).FillWidth().Placeholder(":2222")
					})
					ui.Field(c, "协议", func() {
						ui.Select(c, &a.form.FwdProto, []string{"tcp", "udp"}).FillWidth()
					})
				})
			})

			divider(c)

			formSection(c, "② 转发到", "连接本机端口时，实际连到这些地址（多个即负载均衡）", func() {
				ui.TextArea(c, &a.form.Targets).Lines(3, 6).Placeholder("192.168.1.10:22")
			})

			divider(c)

			formSection(c, "③ 名称", "只用于列表显示", func() {
				ui.TextInput(c, &a.form.Name).Placeholder("如：家里 NAS")
			})
		})
	})
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
