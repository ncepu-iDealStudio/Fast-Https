package cmd

import (
	"fast-https/cmd/internal/cli"

	"github.com/spf13/cobra"
)

// 帮助文案的实现已迁到 cmd/internal/cli。这里只保留 cmd 包内的旧名字，
// 让几十个调用点不必与迁移同步改动；迁移收尾时这些桥接会整体删除。

// Lang 是帮助信息的语言标识（别名，实现在 internal/cli）。
type Lang = cli.Lang

const (
	LangZH  = cli.LangZH
	LangEN  = cli.LangEN
	LangEnv = cli.LangEnv
)

// t 返回当前语言下的文本。
func t(zh, en string) string { return cli.T(zh, en) }

// CurrentLang 返回当前生效的语言。
func CurrentLang() Lang { return cli.CurrentLang() }

// resolveLang 按 --lang > FAST_HTTPS_LANG > 默认中文 决定语言。
func resolveLang(args []string) Lang { return cli.ResolveLang(args) }

// localizeCobra 本地化 cobra 自己生成的那部分文案。
func localizeCobra(root *cobra.Command) { cli.LocalizeCobra(root) }

// walkCommands 深度遍历命令树。
func walkCommands(c *cobra.Command, fn func(*cobra.Command)) { cli.WalkCommands(c, fn) }
