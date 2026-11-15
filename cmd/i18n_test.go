package cmd

import (
	"bytes"
	"fast-https/cmd/internal/cli"
	"fast-https/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// ===== merged from text_test.go =====
// runHelp 用给定语言建树并输出帮助。SetOut 设在根上，子命令会继承。
func runHelp(t *testing.T, lang Lang, args ...string) string {
	t.Helper()
	root := RootCmdForLang(lang)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return buf.String()
}

// hasHan 判断文本里是否有汉字。用它比对比整句翻译更稳：
// 翻译措辞可以改，但"中文帮助里不该只剩英文"这条不会变。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestHelpDefaultsToChinese(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })
	root := RootCmdForLang(LangZH)

	if !hasHan(root.Short) {
		t.Fatalf("根命令说明应为中文，实际 %q", root.Short)
	}
	for _, sub := range root.Commands() {
		if !hasHan(sub.Short) {
			t.Errorf("命令 %q 的说明还是英文：%q", sub.Name(), sub.Short)
		}
	}
}

func TestHelpEnglishVersion(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })
	root := RootCmdForLang(LangEN)

	if hasHan(root.Short) {
		t.Fatalf("--lang en 时根命令说明不应含中文，实际 %q", root.Short)
	}
	for _, sub := range root.Commands() {
		if hasHan(sub.Short) {
			t.Errorf("--lang en 时命令 %q 的说明仍含中文：%q", sub.Name(), sub.Short)
		}
	}
}

// TestEveryCommandTranslated 兜住以后新增命令忘记写中文的情况：
// 只写了一个参数的 cli.T(zh) 在英文模式下会露出中文，反过来也是。
func TestEveryCommandTranslated(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })

	for _, sub := range RootCmdForLang(LangZH).Commands() {
		if sub.Short == "" {
			t.Errorf("命令 %q 缺少说明", sub.Name())
			continue
		}
		if !hasHan(sub.Short) {
			t.Errorf("命令 %q 缺少中文说明", sub.Name())
		}
	}
}

func TestRootHelpIsLocalized(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })

	// 根命令没有继承来的 flag，所以「全局选项」只在子命令的帮助里出现。
	zh := runHelp(t, LangZH, "--help")
	for _, want := range []string{"用法：", "可用命令：", "选项："} {
		if !strings.Contains(zh, want) {
			t.Errorf("中文帮助缺少 %q\n---\n%s", want, zh)
		}
	}
	for _, banned := range []string{"Available Commands:", "Usage:", "Flags:"} {
		if strings.Contains(zh, banned) {
			t.Errorf("中文帮助里混进了英文 %q", banned)
		}
	}

	en := runHelp(t, LangEN, "--help")
	for _, want := range []string{"Usage:", "Available Commands:", "Flags:"} {
		if !strings.Contains(en, want) {
			t.Errorf("英文帮助缺少 %q\n---\n%s", want, en)
		}
	}
}

func TestSubcommandHelpIsLocalized(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })

	zh := runHelp(t, LangZH, "site", "add", "--help")
	for _, flag := range []string{"--domain", "--root", "--tls", "--dry-run"} {
		if !strings.Contains(zh, flag) {
			t.Errorf("site add 帮助缺少 %s\n---\n%s", flag, zh)
		}
	}
	if !strings.Contains(zh, "只生成差异，不应用") {
		t.Errorf("--dry-run 的说明没有翻成中文\n---\n%s", zh)
	}
	if strings.Contains(zh, "show the diff only") {
		t.Errorf("中文帮助里出现了英文 --dry-run 说明\n---\n%s", zh)
	}
	// 子命令继承根命令的 --lang，这一段标题也要是中文。
	if !strings.Contains(zh, "全局选项") {
		t.Errorf("子命令帮助缺少「全局选项」\n---\n%s", zh)
	}
}

func TestResolveLang(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  string
		want Lang
	}{
		{name: "默认中文", want: LangZH},
		{name: "--lang en", args: []string{"--lang", "en"}, want: LangEN},
		{name: "--lang=en", args: []string{"--lang=en_US"}, want: LangEN},
		{name: "--lang zh", args: []string{"--lang", "zh"}, want: LangZH},
		{name: "环境变量", env: "en", want: LangEN},
		{name: "环境变量中文", env: "zh_CN", want: LangZH},
		{name: "参数优先于环境变量", args: []string{"--lang", "zh"}, env: "en", want: LangZH},
		{name: "无法识别的取值回退默认", args: []string{"--lang", "klingon"}, env: "klingon", want: LangZH},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(LangEnv, c.env)
			if got := cli.ResolveLang(c.args); got != c.want {
				t.Fatalf("cli.ResolveLang(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

// findSubCmd 按名字取子命令，取不到直接失败。
func findSubCmd(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("%s 下没有子命令 %q", parent.Name(), name)
	return nil
}

// flagNames 返回命令自身可见 flag 的名字（不含 cobra 补的 -h/--help）。
func flagNames(c *cobra.Command) []string {
	var names []string
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
}

// TestNoShellCompletionCommand：cobra 默认会挂一个 `completion` 子命令，
// 它生成的是 shell 补全脚本，与本项目无关，已显式关掉。
func TestNoShellCompletionCommand(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })

	for _, sub := range RootCmdForLang(LangZH).Commands() {
		if sub.Name() == "completion" {
			t.Fatal("不应出现 cobra 自带的 completion 子命令")
		}
	}
}

// TestAcmeWizardKeepsFlagsMinimal 守住「跑一次向导不需要记参数」：
// acme 只剩 --config，配置项都在问答里问；脚本化走 acme setup。
func TestAcmeWizardKeepsFlagsMinimal(t *testing.T) {
	t.Cleanup(func() { cli.SetLang(cli.LangZH) })

	acme := findSubCmd(t, RootCmdForLang(LangZH), "acme")

	if got := strings.Join(flagNames(acme), ","); got != "config" {
		t.Fatalf("acme 的 flag = [%s]，只应保留 config", got)
	}

	setup := findSubCmd(t, acme, "setup")
	for _, want := range []string{"email", "ca", "challenge", "non-interactive", "storage"} {
		if setup.Flags().Lookup(want) == nil {
			t.Errorf("acme setup 缺少 --%s，脚本化路径要靠它", want)
		}
	}
}

// ===== merged from compat_test.go =====
// 桌面端与 systemd 依赖下面这些旧命令的输出与退出码，CLI 改造期间必须保持不变。
// 这里锁住的是对外契约，不是实现细节。

// config path --json 原有五个键保持不变：桌面端按键解析，多一个 state 不影响。
func TestLegacyConfigPathJSONKeys(t *testing.T) {
	out, _, code := runCLI(t, "config", "path", "--json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}

	doc := decodeOneDocument(t, out)
	legacy := []string{"cert", "config", "log", "pid", "root_crt"}
	for _, k := range legacy {
		if s, _ := doc[k].(string); s == "" {
			t.Errorf("%s 不应为空", k)
		}
	}
	if s, _ := doc["state"].(string); s == "" {
		t.Fatal("state 不应为空")
	}
	if len(doc) != 6 {
		var keys []string
		for k := range doc {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("config path --json 的键 = %v, want 原五个加上 state", keys)
	}
}

// config path 的文本形态也是契约：每行 "名字 路径"。
func TestLegacyConfigPathText(t *testing.T) {
	out, _, code := runCLI(t, "config", "path")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, name := range []string{"config", "cert", "log", "pid", "root_crt", "state"} {
		if !strings.Contains(out, name) {
			t.Errorf("文本输出缺少 %q：\n%s", name, out)
		}
	}
}

// 桌面端取 version 输出的第一行当版本号显示。
func TestLegacyVersionOutput(t *testing.T) {
	out, _, code := runCLI(t, "version")
	want := "fast-https " + config.CURRENT_VERSION + "\n"
	if code != 0 || out != want {
		t.Fatalf("exit=%d out=%q, want %q", code, out, want)
	}
}

func skipIfPidFilePresent(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(config.PID_FILE); err == nil {
		t.Skipf("%s 已存在，跳过依赖「没有 pid 文件」的用例", config.PID_FILE)
	}
}

// status 在没有 pid 文件时返回错误（退出码 1），文案含 "not running"。
func TestLegacyStatusWhenNotRunning(t *testing.T) {
	skipIfPidFilePresent(t)

	_, errOut, code := runCLI(t, "status")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(errOut, "Error: ") || !strings.Contains(errOut, "not running") {
		t.Fatalf("stderr = %q", errOut)
	}
}

// stop 在没有 pid 文件时按「没在跑」处理并成功：桌面端 stopServer 依赖这一点。
func TestLegacyStopWhenNotRunning(t *testing.T) {
	skipIfPidFilePresent(t)

	_, _, code := runCLI(t, "stop")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}
