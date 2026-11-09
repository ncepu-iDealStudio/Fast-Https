package cmd

import (
	"fast-https/cmd/internal/sitecmd"

	"github.com/spf13/cobra"
)

// site 命令组已迁到 cmd/internal/sitecmd。这里保留 cmd 包内的装配入口。

func newSiteCmd() *cobra.Command { return sitecmd.NewSiteCmd() }
