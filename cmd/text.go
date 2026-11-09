package cmd

import "fast-https/cmd/internal/cli"

// 帮助文案的实现已迁到 cmd/internal/cli。root.go 与外部调用方仍按
// cmd.Lang 使用，这里保留类型别名与常量，避免调用方跟着改。

// Lang 是帮助信息的语言标识（别名，实现在 internal/cli）。
type Lang = cli.Lang

const (
	LangZH  = cli.LangZH
	LangEN  = cli.LangEN
	LangEnv = cli.LangEnv
)
