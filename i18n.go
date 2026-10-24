package main

import (
	"strings"

	"github.com/egoist/mygo"
)

// langZh 记录当前界面语言是否为中文。
// 菜单与托盘的文案靠它在中英之间切换，跟随系统语言。
var langZh = true

// detectLang 依据应用当前语言更新 langZh。
// Locale 在应用就绪前为空时，退回系统首选语言判断。
func detectLang() {
	if tag := mygo.App.Locale(); tag != "" {
		langZh = strings.HasPrefix(strings.ToLower(tag), "zh")
		return
	}
	for _, p := range mygo.App.PreferredLanguages() {
		if strings.HasPrefix(strings.ToLower(p), "zh") {
			langZh = true
			return
		}
	}
	langZh = false
}

// currentLocale 返回当前语言的简短描述，用于启动日志排查。
func currentLocale() (locale string, zh bool) {
	detectLang()
	return mygo.App.Locale(), langZh
}

// tr 在中英两套文案间选择；其余语言回退英文。
func tr(zh, en string) string {
	if langZh {
		return zh
	}
	return en
}
