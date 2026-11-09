package cmd

import (
	"io"

	"fast-https/cmd/internal/acmecmd"
	"fast-https/cmd/internal/cli"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// acme 命令组已迁到 cmd/internal/acmecmd。这里保留 cmd 包内的旧名字：
// newAcmeCmd 供根命令注册，另外三个是 site 向导仍在用的终端与校验辅助
// （后续迁 sitecmd 时会一并改为直接调 cli）。

func newAcmeCmd() *cobra.Command { return acmecmd.NewAcmeCmd() }

func applyStdio(form *huh.Form, in io.Reader, out io.Writer, accessible bool) {
	cli.ApplyStdio(form, in, out, accessible)
}

func interactiveAvailable() bool { return cli.InteractiveAvailable() }

func validateNotBlank(v string) error { return cli.ValidateNotBlank(v) }
