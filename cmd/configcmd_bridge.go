package cmd

import (
	"fast-https/cmd/internal/configcmd"

	"github.com/spf13/cobra"
)

// config 命令组的实现已迁到 cmd/internal/configcmd。这里只保留装配入口与
// 少量仍被 cmd 测试引用的符号。

func newConfigCmd() *cobra.Command { return configcmd.NewConfigCmd() }

// RuntimePaths 是运行时路径集合（别名，实现在 internal/configcmd）。
type RuntimePaths = configcmd.RuntimePaths

// ResolvePaths 返回 config / cert / log / pid / root_crt 的绝对路径。
func ResolvePaths() RuntimePaths { return configcmd.ResolvePaths() }
