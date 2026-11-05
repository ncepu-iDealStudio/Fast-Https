package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"fast-https/cmd"
	"fast-https/config"
)

// langs 是必须都覆盖到的两种语言。
var langs = []cmd.Lang{cmd.LangZH, cmd.LangEN}

func TestRenderCoversEveryCommand(t *testing.T) {
	for _, lang := range langs {
		got := Render(lang, config.CURRENT_VERSION)
		for _, sub := range subcommands(cmd.RootCmdForLang(lang)) {
			if !strings.Contains(got, ".B "+escape(sub.Name())+"\n") {
				t.Errorf("[%s] man page 缺少子命令 %q\n---\n%s", lang, sub.Name(), got)
			}
			if sub.Short != "" && !strings.Contains(got, escape(sub.Short)) {
				t.Errorf("[%s] man page 缺少子命令 %q 的说明", lang, sub.Name())
			}
		}
	}
}

func TestRenderHasRequiredSections(t *testing.T) {
	for _, lang := range langs {
		got := Render(lang, config.CURRENT_VERSION)
		for _, want := range []string{
			".TH FAST\\-HTTPS 1",
			".SH NAME",
			".SH SYNOPSIS",
			".SH DESCRIPTION",
			".SH COMMANDS",
			".SH FILES",
			".SH EXAMPLES",
			`/etc/fast\-https/fast\-https.json`,
			`/var/log/fast\-https/`,
			`/usr/lib/systemd/system/fast\-https.service`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("[%s] man page 缺少 %q\n---\n%s", lang, want, got)
			}
		}
	}
}

func TestRenderHasNoDate(t *testing.T) {
	// 输出里带日期会让同一份定义每次生成的结果不同，一致性测试也就无从比较。
	for _, lang := range langs {
		if got := Render(lang, "V1.2.3"); !strings.Contains(got, `.TH FAST\-HTTPS 1 ""`) {
			t.Errorf("[%s] man page 的 .TH 日期字段应为空:\n%s", lang, got)
		}
	}
}

// TestRenderFollowsLanguage 保证两份手册页确实不是一个文件复制两遍：
// 固定内容（NAME、FILES、EXAMPLES）也要跟着语言走。
func TestRenderFollowsLanguage(t *testing.T) {
	zh := Render(cmd.LangZH, config.CURRENT_VERSION)
	en := Render(cmd.LangEN, config.CURRENT_VERSION)

	if !hasHan(zh) {
		t.Error("中文手册页里没有任何汉字")
	}
	for _, want := range []string{"主配置文件", "启动服务；systemd 与容器内会保持前台运行"} {
		if !strings.Contains(zh, escape(want)) {
			t.Errorf("中文手册页缺少 %q", want)
		}
	}

	if hasHan(en) {
		t.Error("英文手册页里混进了中文")
	}
	for _, want := range []string{"high performance web server", "main configuration file"} {
		if !strings.Contains(en, want) {
			t.Errorf("英文手册页缺少 %q", want)
		}
	}
}

// hasHan 判断文本里是否有汉字。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// TestManPageIsUpToDate 阻止改了命令定义却忘记重新生成 man page。
func TestManPageIsUpToDate(t *testing.T) {
	for _, item := range []struct {
		path string
		lang cmd.Lang
	}{
		{defaultOut, cmd.LangZH},
		{defaultEnOut, cmd.LangEN},
	} {
		path := filepath.Join("..", "..", item.path)

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v（先执行 go run ./tools/manpage）", path, err)
		}

		got := Render(item.lang, config.CURRENT_VERSION)
		if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Fatalf("%s 与命令定义不一致，请执行 go run ./tools/manpage 重新生成", path)
		}
	}
}
