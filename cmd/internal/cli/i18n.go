// Package cli 汇集所有 CLI 命令共用的设施：帮助文案的国际化、错误与退出码、
// 文本/JSON 输出、通用 flag 解析。
//
// 放在 cmd/internal 下是为了把可见范围收到 cmd 子树：业务包（management、
// modules、runtime）不应该依赖 CLI 的 i18n 与退出码，一旦有人反着 import，
// 编译就会失败。
package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Lang 是帮助信息的语言标识。
type Lang string

const (
	LangZH Lang = "zh"
	LangEN Lang = "en"
)

// LangEnv 是语言的环境变量入口，方便脚本里一次性切换。
const LangEnv = "FAST_HTTPS_LANG"

// currentLang 由 SetLang 在构造命令树前写入，此后只读。
// cobra 的 Short/Long 是字符串字段，建树时给哪份就一直是哪份，运行时再改也没用。
var currentLang = LangZH

// SetLang 在构造命令树之前设置语言。
func SetLang(lang Lang) { currentLang = lang }

// CurrentLang 返回当前生效的语言。
func CurrentLang() Lang { return currentLang }

// T 返回当前语言下的文本。中英两版直接写在调用处：
// 没有 key 表，也就不存在 key 写错或漏翻译导致的空帮助。
func T(zh, en string) string {
	if currentLang == LangEN {
		return en
	}
	return zh
}

// ResolveLang 按 --lang > FAST_HTTPS_LANG > 默认中文 的顺序决定语言。
// args 不含程序名。
func ResolveLang(args []string) Lang {
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

// LocalizeCobra 处理 cobra 自己生成的那部分文案：用法模板、help 命令，
// 以及每条命令上 -h 的说明。英文版用 cobra 默认值，不用管。
func LocalizeCobra(root *cobra.Command) {
	if currentLang == LangEN {
		return
	}

	root.SetUsageTemplate(zhUsageTemplate)

	// help 已在建树时显式生成，这里只改文案。
	for _, sub := range root.Commands() {
		if sub.Name() != "help" {
			continue
		}
		sub.Short = "查看任意命令的帮助"
		sub.Long = "逐级给出命令的帮助信息。\n" +
			"直接执行 `" + root.DisplayName() + " help <命令>` 查看指定命令的详细说明。"
	}

	WalkCommands(root, localizeHelpFlag)
}

// localizeHelpFlag 把 -h 的说明换成中文。cobra 只在命令自身没有 help flag 时
// 才会补默认的那个，所以先占位就能生效。
func localizeHelpFlag(c *cobra.Command) {
	if c.Flags().Lookup("help") != nil {
		return
	}
	c.Flags().BoolP("help", "h", false, "显示帮助")
}

// WalkCommands 深度遍历命令树，fn 对每个命令调用一次。
func WalkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		WalkCommands(sub, fn)
	}
}
