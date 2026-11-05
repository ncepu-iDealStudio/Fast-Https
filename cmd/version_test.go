package cmd

import (
	"bytes"
	"strings"
	"testing"

	"fast-https/config"
)

// runCmd 用给定语言建树、执行一次命令，返回 stdout 与错误。
func runCmd(t *testing.T, lang Lang, args ...string) (string, error) {
	t.Helper()
	root := RootCmdForLang(lang)
	t.Cleanup(func() { currentLang = LangZH })

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	got, err := runCmd(t, LangZH, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}

	// 输出要能直接粘进 issue：一行，带上程序名，版本号与二进制一致。
	want := "fast-https " + config.CURRENT_VERSION + "\n"
	if got != want {
		t.Fatalf("version 输出 = %q, want %q", got, want)
	}
}

// 版本号不随帮助语言变化，否则脚本解析就得处理两种格式。
func TestVersionOutputIsLanguageIndependent(t *testing.T) {
	zh, err := runCmd(t, LangZH, "version")
	if err != nil {
		t.Fatalf("version(zh): %v", err)
	}
	en, err := runCmd(t, LangEN, "version")
	if err != nil {
		t.Fatalf("version(en): %v", err)
	}
	if zh != en {
		t.Fatalf("中英输出不一致：%q vs %q", zh, en)
	}
}

func TestVersionRejectsExtraArgs(t *testing.T) {
	if _, err := runCmd(t, LangZH, "version", "extra"); err == nil {
		t.Fatal("version 不接受位置参数")
	}
}

// version 必须出现在顶层命令列表里，且说明跟着语言走。
func TestVersionListedInHelp(t *testing.T) {
	zh, err := runCmd(t, LangZH, "--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(zh, "打印版本号") {
		t.Fatalf("中文帮助里没有 version：\n%s", zh)
	}

	en, err := runCmd(t, LangEN, "--help")
	if err != nil {
		t.Fatalf("--help(en): %v", err)
	}
	if !strings.Contains(en, "print the version") {
		t.Fatalf("英文帮助里没有 version：\n%s", en)
	}
}
