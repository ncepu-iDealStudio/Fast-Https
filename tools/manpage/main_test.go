package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/cmd"
	"fast-https/config"
)

func TestRenderCoversEveryCommand(t *testing.T) {
	got := Render(cmd.RootCmd(), config.CURRENT_VERSION)

	for _, sub := range subcommands(cmd.RootCmd()) {
		if !strings.Contains(got, ".B "+escape(sub.Name())+"\n") {
			t.Errorf("man page 缺少子命令 %q\n---\n%s", sub.Name(), got)
		}
		if sub.Short != "" && !strings.Contains(got, escape(sub.Short)) {
			t.Errorf("man page 缺少子命令 %q 的说明", sub.Name())
		}
	}
}

func TestRenderHasRequiredSections(t *testing.T) {
	got := Render(cmd.RootCmd(), config.CURRENT_VERSION)

	for _, want := range []string{
		".TH FAST\\-HTTPS 1",
		".SH NAME",
		".SH SYNOPSIS",
		".SH DESCRIPTION",
		".SH COMMANDS",
		".SH FILES",
		".SH EXAMPLES",
		`/usr/share/fast\-https/config/fast\-https.json`,
		`/usr/lib/systemd/system/fast\-https.service`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("man page 缺少 %q\n---\n%s", want, got)
		}
	}
}

func TestRenderHasNoDate(t *testing.T) {
	// 输出里带日期会让同一份定义每次生成的结果不同，一致性测试也就无从比较。
	if got := Render(cmd.RootCmd(), "V1.2.3"); !strings.Contains(got, `.TH FAST\-HTTPS 1 ""`) {
		t.Errorf("man page 的 .TH 日期字段应为空:\n%s", got)
	}
}

// TestManPageIsUpToDate 阻止改了命令定义却忘记重新生成 man page。
func TestManPageIsUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", defaultOut)

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v（先执行 go run ./tools/manpage）", path, err)
	}

	got := Render(cmd.RootCmd(), config.CURRENT_VERSION)
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatal("man page 与命令定义不一致，请执行 go run ./tools/manpage 重新生成")
	}
}
