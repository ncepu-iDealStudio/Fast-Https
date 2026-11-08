package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"fast-https/config"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func addConfigFlag(c *cobra.Command) {
	c.Flags().String("config", config.CONFIG_FILE_PATH, t("主配置文件", "main config file"))
}

// printWarnings 在文本模式下把告警写到 stderr，不污染 stdout。
func printWarnings(w io.Writer, warnings []string) {
	for _, msg := range warnings {
		fmt.Fprintf(w, "warning: %s\n", msg)
	}
}

func printIndentedJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newConfigShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show",
		Short: t("显示磁盘上的配置（敏感值脱敏）", "show the on-disk configuration (secrets redacted)"),
		Long: t(
			"显示磁盘上的主配置与 http.include 站点，口令、令牌等敏感值一律脱敏，没有显示明文的选项。\n\n"+
				"来源是磁盘，不是运行中进程实际生效的配置；config_dirty_hint 只是按修改时间做的提示。\n"+
				"--section 可选：http、acme、security、waf、sites。",
			"show the main config and the sites under http.include; passwords, tokens and other\n"+
				"secrets are always redacted and there is no option to reveal them.\n\n"+
				"The source is the disk, not the configuration the running process has applied;\n"+
				"config_dirty_hint is only a modification-time hint.\n"+
				"--section accepts: http, acme, security, waf, sites."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			section, _ := cmd.Flags().GetString("section")

			// 残留的 pid 文件说明不了「进程启动之后配置有没有改过」，只在进程确实在跑时才给提示。
			pidFile := config.PID_FILE
			if state, _, _ := InspectProcess(pidFile); state != ProcessRunning {
				pidFile = ""
			}

			data, warnings, err := view.ConfigShow(view.ConfigShowInput{
				MainPath: path, Section: section, PidFile: pidFile,
			})
			if err != nil {
				return fromViewError("config.show", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "config.show", data, warnings)
			}
			printWarnings(cmd.ErrOrStderr(), warnings)
			return printIndentedJSON(cmd.OutOrStdout(), data)
		},
	}
	addConfigFlag(c)
	c.Flags().String("section", "", t("只显示某一段：http、acme、security、waf、sites", "show one section only: http, acme, security, waf, sites"))
	return c
}

func newConfigValidateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "validate",
		Short: t("校验配置，不应用", "validate a config without applying it"),
		Long: t(
			"校验当前配置，或用 --candidate 校验一个候选主配置文件；只校验，不应用、不重载。\n"+
				"校验失败时退出码为 5，原因在错误信息里。\n\n"+
				"候选文件目前只覆盖主配置本身：http.include 与证书路径仍按运行期布局解析。",
			"validate the current config, or a candidate main config file with --candidate; it never\n"+
				"applies or reloads anything. A failed validation exits with code 5.\n\n"+
				"A candidate currently covers the main file only: http.include and certificate paths\n"+
				"are still resolved against the running layout."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			candidate := false
			if c, _ := cmd.Flags().GetString("candidate"); c != "" {
				path, candidate = c, true
			}

			data, warnings, err := view.ConfigValidate(path, candidate)
			if err != nil {
				return fromViewError("config.validate", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "config.validate", data, warnings)
			}
			printWarnings(cmd.ErrOrStderr(), warnings)
			fmt.Fprintf(cmd.OutOrStdout(), t("配置校验通过：%s\n", "config is valid: %s\n"), path)
			return nil
		},
	}
	addConfigFlag(c)
	c.Flags().String("candidate", "", t("候选主配置文件（只校验，不应用）", "candidate main config file (validated only, never applied)"))
	return c
}
