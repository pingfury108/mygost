package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gopkg.in/yaml.v3"
	"mygost/internal/profiles"
	"mygost/internal/settings"
)

var kindNames = []string{"代理", "端口转发", "YAML"}
var kindValues = []profiles.Kind{profiles.KindProxy, profiles.KindForward, profiles.KindCustom}

// viewProfileForm 新建/编辑表单（子页面，窄窗口单列）。
func (a *app) viewProfileForm(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(14).Children(func() {
		if ui.Segmented(c, &a.formKind, kindNames...).Changed() {
			a.syncFormKind()
		}

		switch kindValues[a.formKind] {
		case profiles.KindProxy:
			a.proxyForm(c)
			a.viewUpstream(c)
		case profiles.KindForward:
			a.forwardForm(c)
		case profiles.KindCustom:
			a.customForm(c)
		}

		// 底部操作
		ui.Row(c).Gap(10).Padding(4, 0).Children(func() {
			ui.Button(c, "取消").Grow(1).OnClick(a.goMain)
			ui.PrimaryButton(c, "保存").Grow(1).OnClick(a.saveEdit)
		})
	})
}

// formName 名称字段（各类型共用）。
func formName(c *ui.Context, a *app) {
	ui.Field(c, "名称", func() {
		ui.TextInput(c, &a.form.Name).Placeholder("如：本机代理")
	})
}

func (a *app) proxyForm(c *ui.Context) {
	card(c).Children(func() {
		ui.Column(c).Fill().Gap(12).Children(func() {
			formName(c, a)
			ui.Row(c).Gap(12).Children(func() {
				ui.Field(c, "监听地址", func() {
					ui.TextInput(c, &a.form.Listen).Grow(1).Placeholder(":8080")
				})
				ui.Field(c, "协议", func() {
					ui.Select(c, &a.form.Proto, []string{"http", "socks5", "relay"}).Width(120)
				})
			})
			ui.Row(c).Gap(12).Children(func() {
				ui.Field(c, "用户名（可选）", func() {
					ui.TextInput(c, &a.form.User).Grow(1)
				})
				ui.Field(c, "密码（可选）", func() {
					ui.TextInput(c, &a.form.Password).Grow(1).Password()
				})
			})
		})
	})
}

func (a *app) forwardForm(c *ui.Context) {
	card(c).Children(func() {
		ui.Column(c).Fill().Gap(12).Children(func() {
			formName(c, a)
			ui.Row(c).Gap(12).Children(func() {
				ui.Field(c, "监听地址", func() {
					ui.TextInput(c, &a.form.Listen).Grow(1).Placeholder(":2222")
				})
				ui.Field(c, "协议", func() {
					ui.Select(c, &a.form.FwdProto, []string{"tcp", "udp"}).Width(120)
				})
			})
			ui.Field(c, "目标地址（每行一个，多个为负载均衡）", func() {
				ui.TextArea(c, &a.form.Targets).Lines(3, 6).Placeholder("192.168.1.1:22")
			})
		})
	})
}

func (a *app) customForm(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "直接编辑 gost.yml（完整能力）。").TextColor(t.TextMuted)
	ui.Box(c).Fill().Height(360).Radius(10).Background(t.Surface).Children(func() {
		if a.customBuf != nil {
			editor.View(c, a.customBuf).Fill()
		}
	})
}

// viewUpstream 代理档的可选上游转发链。
func (a *app) viewUpstream(c *ui.Context) {
	ui.Collapsible(c, "上游转发链（经由另一台代理）", &a.formUpOpen, func() {
		card(c).Children(func() {
			ui.Column(c).Fill().Gap(12).Children(func() {
				settingRow(c, "启用上游", "流量将经由该上游转发", func() {
					ui.Switch(c, &a.form.UpEnabled)
				})
				if a.form.UpEnabled {
					ui.Row(c).Gap(12).Children(func() {
						ui.Field(c, "协议", func() {
							ui.Select(c, &a.form.UpProto, []string{"socks5", "http"}).Width(120)
						})
						ui.Field(c, "地址", func() {
							ui.TextInput(c, &a.form.UpAddr).Grow(1).Placeholder("1.2.3.4:1080")
						})
					})
					ui.Row(c).Gap(12).Children(func() {
						ui.Field(c, "用户名（可选）", func() {
							ui.TextInput(c, &a.form.UpUser).Grow(1)
						})
						ui.Field(c, "密码（可选）", func() {
							ui.TextInput(c, &a.form.UpPass).Grow(1).Password()
						})
					})
				}
			})
		})
	})
}

func (a *app) formTitle() string {
	if a.form.ID == "" {
		return "新建配置"
	}
	return "编辑配置"
}

// startNew 进入新建表单。
func (a *app) startNew() {
	a.form = profiles.Profile{
		Kind: profiles.KindProxy, Proto: "http",
		FwdProto: "tcp", UpProto: "socks5",
	}
	a.formKind = 0
	a.formUpOpen = false
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
	a.customBuf = nil
	if p.Kind == profiles.KindCustom {
		a.customBuf = newYAMLBuffer(p.YAML)
	}
	a.page = "editor"
}

// syncFormKind 在切换类型时准备 custom 编辑器缓冲。
func (a *app) syncFormKind() {
	if kindValues[a.formKind] == profiles.KindCustom {
		src := a.form.YAML
		if src == "" {
			if y, err := a.form.BuildYAML(); err == nil {
				src = string(y)
			}
		}
		a.customBuf = newYAMLBuffer(src)
	}
}

// saveEdit 校验并保存。
func (a *app) saveEdit() {
	a.form.Kind = kindValues[a.formKind]
	if a.form.Kind == profiles.KindCustom && a.customBuf != nil {
		a.form.YAML = a.customBuf.Buffer.String()
		if !yamlValid(a.form.YAML) {
			a.toast("YAML 语法有误，请检查", "error")
			return
		}
	}
	if _, err := a.ps.Save(&a.form); err != nil {
		a.toast("保存失败: "+err.Error(), "error")
		return
	}
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
