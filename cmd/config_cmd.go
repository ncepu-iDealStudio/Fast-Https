package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"fast-https/config"
	"fast-https/management/cfgdiff"
	"fast-https/management/cfgver"
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
			"校验当前配置，或用 --candidate 校验一棵候选目录（fast-https.json + conf.d）。\n"+
				"只校验，不应用、不重载。校验失败时退出码为 5。候选路径含 .. 或符号链接逃出目录时退出码为 2。",
			"validate the current config, or a candidate directory (fast-https.json + conf.d) with --candidate.\n"+
				"It never applies or reloads anything. A failed validation exits with code 5.\n"+
				"A candidate path containing .. or a symlink that escapes the tree exits with code 2."),
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
				if errors.Is(err, config.ErrCandidatePath) {
					return usageError(err)
				}
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
	c.Flags().String("candidate", "", t("候选配置目录（只校验，不应用）", "candidate config directory (validated only, never applied)"))
	return c
}

func newConfigVersionsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "versions",
		Short: t("列出配置版本快照", "list configuration version snapshots"),
		Long: t(
			"列出状态目录里的配置版本。active 不靠标志位，而是当前磁盘配置树的哈希与快照哈希相同的那一个。\n"+
				"都不相同则 drift 为 true。快照不含证书与私钥。",
			"List configuration versions in the state directory. active is not stored as a flag:\n"+
				"the version whose tree hash matches the on-disk config is active. If none match, drift is true.\n"+
				"Snapshots never include certificates or private keys."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			data, err := cfgver.List(stateDir(), path)
			if err != nil {
				return fromStateError("config.versions", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "config.versions", data, nil)
			}
			if len(data.Versions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no versions)")
				return nil
			}
			for _, v := range data.Versions {
				mark := ""
				if v.Active {
					mark = " active"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s\n", v.Version, mark, v.CreatedAt, v.Summary)
			}
			if data.Drift {
				fmt.Fprintln(cmd.OutOrStdout(), "drift: true")
			}
			return nil
		},
	}
	addConfigFlag(c)
	return c
}

func newConfigDiffCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diff",
		Short: t("比较当前配置与候选配置", "compare the current config with a candidate"),
		Long: t(
			"比较当前配置与 --candidate（目录、主配置文件或版本号，例如 v001）。只展示差异，不应用。\n"+
				"敏感键只显示 changed，不输出新旧值。无法解析时失败，不会用空差异冒充一致。\n"+
				"候选为目录或版本快照时比较整棵 JSON 树（主文件 + conf.d）。",
			"Compare the current config with --candidate (a directory, main file, or version such as v001).\n"+
				"It only shows differences and never applies them. Sensitive keys are reported as changed\n"+
				"without old or new values. A parse failure is an error, never an empty diff.\n"+
				"A directory or version snapshot is compared as a whole JSON tree (main file + conf.d)."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			current, _ := cmd.Flags().GetString("current")
			if strings.TrimSpace(current) == "" {
				current, _ = cmd.Flags().GetString("config")
			}
			candidate, _ := cmd.Flags().GetString("candidate")
			if strings.TrimSpace(candidate) == "" {
				return usageError(fmt.Errorf("--candidate is required"))
			}
			if err := rejectCandidateEscape(candidate); err != nil {
				return err
			}
			candPath, err := resolveDiffCandidate(candidate)
			if err != nil {
				return err
			}
			data, err := diffConfig(current, candPath)
			if err != nil {
				return fromViewError("config.diff", err)
			}
			if jsonMode(cmd) {
				return emitOK(cmd, "config.diff", data, nil)
			}
			if data.Identical {
				fmt.Fprintln(cmd.OutOrStdout(), "identical: true")
				return nil
			}
			return printIndentedJSON(cmd.OutOrStdout(), data)
		},
	}
	addConfigFlag(c)
	c.Flags().String("current", "", t("当前配置文件或目录，默认与 --config 相同", "current config file or directory, defaults to --config"))
	c.Flags().String("candidate", "", t("候选配置目录、文件或版本号（必填）", "candidate config directory, file, or version (required)"))
	return c
}

func rejectCandidateEscape(path string) error {
	if _, err := config.ResolveCandidateRoot(path); err != nil {
		if errors.Is(err, config.ErrCandidatePath) {
			return usageError(err)
		}
	}
	return nil
}

func diffConfig(current, candidate string) (cfgdiff.Result, error) {
	candDir := isDir(candidate)
	curDir := isDir(current)
	if candDir || curDir {
		curRoot := current
		if !curDir {
			curRoot = filepath.Dir(current)
		}
		candRoot := candidate
		if !candDir {
			candRoot = filepath.Dir(candidate)
		}
		return cfgdiff.Trees(curRoot, candRoot)
	}
	return cfgdiff.Files(current, candidate)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func resolveDiffCandidate(candidate string) (string, error) {
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	if strings.HasPrefix(candidate, "v") {
		dest := filepath.Join(stateDir().VersionsDir(), candidate)
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			return dest, nil
		}
		p, err := cfgver.MainFile(stateDir(), candidate)
		if err != nil {
			return "", &CLIError{Code: CodeNotFound, Command: "config.diff", Message: "version not found: " + candidate, Cause: err}
		}
		return p, nil
	}
	return "", &CLIError{Code: CodeNotFound, Command: "config.diff", Message: "candidate not found: " + candidate}
}
