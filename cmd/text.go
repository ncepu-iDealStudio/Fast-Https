package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// 帮助文本的两种语言。默认中文：使用者和文档都是中文，英文只在显式要求时出现，
// 免得中文环境里为了看帮助还要多带一个参数。
//
// 切换入口有两个：--lang 优先，其次环境变量 FAST_HTTPS_LANG。
// 语言在构造命令树时定下来，之后只读——cobra 的 Short/Long 是字符串字段，
// 建树时给哪份就一直是哪份，运行时再改也没用。

// Lang 是帮助信息的语言标识。
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

// LangEnv 是语言的环境变量入口，方便脚本里一次性切换。
const LangEnv = "FAST_HTTPS_LANG"

// currentLang 由 RootCmdForLang 在构造命令树前写入，此后只读。
var currentLang = LangZH

// t 返回当前语言下的文本。中英两版直接写在调用处：
// 没有 key 表，也就不存在 key 写错或漏翻译导致的空帮助。
func t(zh, en string) string {
	if currentLang == LangEN {
		return en
	}
	return zh
}

// CurrentLang 返回当前生效的语言。
func CurrentLang() Lang { return currentLang }

// resolveLang 按 --lang > FAST_HTTPS_LANG > 默认中文 的顺序决定语言。
// args 不含程序名。
func resolveLang(args []string) Lang {
	if v := langFromArgs(args); v != "" {
		return v
	}
	if v := normalizeLang(os.Getenv(LangEnv)); v != "" {
		return v
	}
	return LangZH
}

// langFromArgs 只扫一遍参数里有没有 --lang。必须在 cobra 解析之前拿到语言，
// 否则命令树已经按默认语言建好了。
func langFromArgs(args []string) Lang {
	for i, arg := range args {
		if arg == "--lang" && i+1 < len(args) {
			if v := normalizeLang(args[i+1]); v != "" {
				return v
			}
			return ""
		}
		if v, ok := strings.CutPrefix(arg, "--lang="); ok {
			return normalizeLang(v)
		}
	}
	return ""
}

// normalizeLang 认 zh/zh_CN/chinese 与 en/en_US/english，其余一律当作没给。
func normalizeLang(v string) Lang {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case v == "":
		return ""
	case strings.HasPrefix(v, "zh"), v == "cn", v == "chinese":
		return LangZH
	case strings.HasPrefix(v, "en"):
		return LangEN
	}
	return ""
}

// zhUsageTemplate 是 cobra 默认用法模板的中文版，结构保持不变：
// 改的只有标题文字，字段与条件分支一一对应。
const zhUsageTemplate = `用法：{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

别名：
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例：
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

可用命令：{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

其它命令：{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

选项：
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

全局选项：
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

其它帮助主题：{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

使用 "{{.CommandPath}} [command] --help" 查看某个命令的详细用法。{{end}}
`

// localizeCobra 处理 cobra 自己生成的那部分文案：用法模板、help 命令，
// 以及每条命令上 -h 的说明。英文版用 cobra 默认值，不用管。
func localizeCobra(root *cobra.Command) {
	if currentLang == LangEN {
		return
	}

	root.SetUsageTemplate(zhUsageTemplate)

	// help 已在建树时显式生成（见 RootCmdForLang），这里只改文案。
	for _, sub := range root.Commands() {
		if sub.Name() != "help" {
			continue
		}
		sub.Short = "查看任意命令的帮助"
		sub.Long = "逐级给出命令的帮助信息。\n" +
			"直接执行 `" + root.DisplayName() + " help <命令>` 查看指定命令的详细说明。"
	}

	walkCommands(root, localizeHelpFlag)
}

// localizeHelpFlag 把 -h 的说明换成中文。cobra 只在命令自身没有 help flag 时
// 才会补默认的那个，所以先占位就能生效。
func localizeHelpFlag(c *cobra.Command) {
	if c.Flags().Lookup("help") != nil {
		return
	}
	c.Flags().BoolP("help", "h", false, "显示帮助")
}

// walkCommands 深度遍历命令树，fn 对每个命令调用一次。
func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}
