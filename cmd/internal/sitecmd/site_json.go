package sitecmd

import (
	"fast-https/cmd/internal/cli"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

// site list / site show 的 --json 分支。文本分支仍在 site.go，输出不变。

func RunListJSON(cmd *cobra.Command) error {
	path, _ := cmd.Flags().GetString("config")
	data, warnings, err := view.SiteList(path)
	if err != nil {
		return cli.FromViewError("site.list", err)
	}
	return cli.EmitOK(cmd, "site.list", data, warnings)
}

func RunShowJSON(cmd *cobra.Command, name string) error {
	path, _ := cmd.Flags().GetString("config")
	merged, _ := cmd.Flags().GetBool("merged")
	data, err := view.SiteShow(path, name, merged)
	if err != nil {
		return cli.FromViewError("site.show", err)
	}
	return cli.EmitOK(cmd, "site.show", data, nil)
}
