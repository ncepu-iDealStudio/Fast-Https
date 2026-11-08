package cmd

import (
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

// site list / site show 的 --json 分支。文本分支仍在 site.go，输出不变。

func runSiteListJSON(cmd *cobra.Command) error {
	path, _ := cmd.Flags().GetString("config")
	data, warnings, err := view.SiteList(path)
	if err != nil {
		return fromViewError("site.list", err)
	}
	return emitOK(cmd, "site.list", data, warnings)
}

func runSiteShowJSON(cmd *cobra.Command, name string) error {
	path, _ := cmd.Flags().GetString("config")
	merged, _ := cmd.Flags().GetBool("merged")
	data, err := view.SiteShow(path, name, merged)
	if err != nil {
		return fromViewError("site.show", err)
	}
	return emitOK(cmd, "site.show", data, nil)
}
