package configcmd

import (
	"context"
	"errors"
	"fast-https/cmd/internal/cli"
	"fast-https/cmd/internal/procctl"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fast-https/config"
	"fast-https/management/cfgdiff"
	"fast-https/management/cfgver"
	"fast-https/management/operation"
	"fast-https/management/svcctl"
	"fast-https/management/txn"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func newConfigShowCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "show",
		Short: cli.T("显示磁盘上的配置（敏感值脱敏）", "show the on-disk configuration (secrets redacted)"),
		Long: cli.T(
			"显示磁盘上的主配置与 http.include 站点，口令、令牌等敏感值一律脱敏，没有显示明文的选项。\n\n"+
				"来源是磁盘，不是运行中进程实际生效的配置；config_dirty_hint 只是按修改时间做的提示。\n"+
				"--section 可选：http、acme、security、tls、proxy、waf、sites。",
			"show the main config and the sites under http.include; passwords, tokens and other\n"+
				"secrets are always redacted and there is no option to reveal them.\n\n"+
				"The source is the disk, not the configuration the running process has applied;\n"+
				"config_dirty_hint is only a modification-time hint.\n"+
				"--section accepts: http, acme, security, tls, proxy, waf, sites."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			section, _ := cmd.Flags().GetString("section")

			// 残留的 pid 文件说明不了「进程启动之后配置有没有改过」，只在进程确实在跑时才给提示。
			pidFile := config.PID_FILE
			if state, _, _ := procctl.InspectProcess(pidFile); state != procctl.ProcessRunning {
				pidFile = ""
			}

			data, warnings, err := view.ConfigShow(view.ConfigShowInput{
				MainPath: path, Section: section, PidFile: pidFile,
			})
			if err != nil {
				return cli.FromViewError("config.show", err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "config.show", data, warnings)
			}
			cli.PrintWarnings(cmd.ErrOrStderr(), warnings)
			return cli.PrintIndentedJSON(cmd.OutOrStdout(), data)
		},
	}
	cli.AddConfigFlag(c)
	c.Flags().String("section", "", cli.T("只显示某一段：http、acme、security、tls、proxy、waf、sites", "show one section only: http, acme, security, tls, proxy, waf, sites"))
	return c
}

func newConfigValidateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "validate",
		Short: cli.T("校验配置，不应用", "validate a config without applying it"),
		Long: cli.T(
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
					return cli.UsageError(err)
				}
				return cli.FromViewError("config.validate", err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "config.validate", data, warnings)
			}
			cli.PrintWarnings(cmd.ErrOrStderr(), warnings)
			fmt.Fprintf(cmd.OutOrStdout(), cli.T("配置校验通过：%s\n", "config is valid: %s\n"), path)
			return nil
		},
	}
	cli.AddConfigFlag(c)
	c.Flags().String("candidate", "", cli.T("候选配置目录（只校验，不应用）", "candidate config directory (validated only, never applied)"))
	return c
}

func newConfigVersionsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "versions",
		Short: cli.T("列出配置版本快照", "list configuration version snapshots"),
		Long: cli.T(
			"列出状态目录里的配置快照，版本号形如 v2026.11.18.1（日期 + 当天序号）。\n"+
				"每成功改一次配置只保存一份结果快照；回滚是把 active 切回旧快照，不会新建版本。\n"+
				"active 不靠标志位，而是当前磁盘配置树的哈希与快照哈希相同的那一个；都不相同则 drift 为 true。\n"+
				"快照包含主配置、conf.d 与服务证书/私钥，不含 ACME 存储目录。\n"+
				"每个版本会附带最近一次产生它的 operation_id 与 command。",
			"List configuration snapshots in the state directory. Versions look like v2026.11.18.1 (date plus a daily counter).\n"+
				"Every successful config change saves one result snapshot; rollback switches active back to an old snapshot\n"+
				"and never creates a version. active is not stored as a flag: the version whose tree hash matches the\n"+
				"on-disk config is active, and if none match, drift is true.\n"+
				"Snapshots include the main config, conf.d and service certificates/keys (not ACME storage).\n"+
				"Each version is annotated with the latest operation_id and command that produced it."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			data, err := cfgver.List(cli.StateDir(), path)
			if err != nil {
				return cli.FromStateError("config.versions", err)
			}
			// 版本历史由「快照 + 操作」共同派生：给每个版本补上最近一次产生它的操作。
			ops, err := operation.Store{Dir: cli.StateDir()}.List(operation.ListFilter{Limit: operation.MaxLimit})
			if err != nil {
				return cli.FromStateError("config.versions", err)
			}
			latest := map[string]operation.Record{}
			for _, op := range ops {
				v, _ := op.Result["version"].(string)
				if v == "" {
					continue
				}
				if _, seen := latest[v]; !seen {
					latest[v] = op
				}
			}
			for i := range data.Versions {
				if op, ok := latest[data.Versions[i].Version]; ok {
					data.Versions[i].OperationID = op.ID
					data.Versions[i].Command = op.Command
				}
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "config.versions", data, nil)
			}
			if len(data.Versions) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), cli.T("（没有版本记录）", "(no versions)"))
				return nil
			}
			for _, v := range data.Versions {
				mark := ""
				if v.Active {
					mark = " active"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s  %s  %s  %s\n", v.Version, mark, v.CreatedAt, v.Command, v.Summary)
			}
			if data.Drift {
				fmt.Fprintln(cmd.OutOrStdout(), cli.T("drift: true（磁盘配置与任何快照都不一致）", "drift: true (on-disk config matches no snapshot)"))
			}
			return nil
		},
	}
	cli.AddConfigFlag(c)
	c.AddCommand(newConfigVersionsDeleteCmd())
	return c
}

func newConfigVersionsDeleteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "delete <version>",
		Short: cli.T("删除一个历史版本快照", "delete a historical version snapshot"),
		Long: cli.T(
			"删除一个版本快照目录。当前 active 版本不能删除；operation 记录保留，不会连带删除历史操作。\n"+
				"版本号形如 v2026.11.18.1；旧的时间戳（20261118-141108-836）与 vNNN 也能识别。\n"+
				"脚本中必须加 --yes；在终端里会先询问 y/N。",
			"Delete a version snapshot directory. The active version cannot be deleted; operation records are kept.\n"+
				"The version looks like v2026.11.18.1; legacy timestamps (20261118-141108-836) and vNNN are also accepted.\n"+
				"Scripts must pass --yes; interactive terminals are asked y/N first. --dry-run only validates and reports."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			dry, _ := cmd.Flags().GetBool("dry-run")
			version := strings.TrimSpace(args[0])
			mapErr := func(err error) error {
				switch {
				case errors.Is(err, cfgver.ErrActiveVersion):
					return &cli.CLIError{Code: cli.CodeConflict, Command: "config.versions.delete", Message: "cannot delete the active version: " + version, Cause: err}
				case errors.Is(err, cfgver.ErrVersionNotFound):
					return &cli.CLIError{Code: cli.CodeNotFound, Command: "config.versions.delete", Message: "version not found: " + version, Cause: err}
				default:
					return cli.FromStateError("config.versions.delete", err)
				}
			}
			// 先校验再确认：版本不存在或仍是 active 时直接报错，不让用户白确认一次。
			if err := cfgver.ValidateDelete(cli.StateDir(), version, path); err != nil {
				return mapErr(err)
			}
			if dry {
				if cli.JSONMode(cmd) {
					return cli.EmitOK(cmd, "config.versions.delete", map[string]any{"version": version, "deleted": false, "dry_run": true}, nil)
				}
				fmt.Fprintf(cmd.OutOrStdout(), cli.T("将删除版本 %s（dry-run，未改动）\n", "would delete version %s (dry-run, nothing changed)\n"), version)
				return nil
			}
			if err := cli.ConfirmWrite(cmd, cli.T("删除版本 "+version, "delete version "+version)); err != nil {
				return err
			}
			if err := cfgver.Delete(cli.StateDir(), version, path); err != nil {
				return mapErr(err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "config.versions.delete", map[string]any{"version": version, "deleted": true}, nil)
			}
			fmt.Fprintf(cmd.OutOrStdout(), cli.T("已删除 %s\n", "deleted %s\n"), version)
			return nil
		},
	}
	cli.AddConfigFlag(c)
	c.Flags().Bool("dry-run", false, cli.T("只校验并显示会删除的版本，不删目录", "validate and show the version that would be deleted"))
	c.Flags().Bool("yes", false, cli.T("确认删除（脚本中非 dry-run 必填；终端中会询问 y/N）", "confirm deletion (required in scripts; interactive terminals are asked y/N)"))
	return c
}

func newConfigDiffCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diff",
		Short: cli.T("比较当前配置与候选配置", "compare the current config with a candidate"),
		Long: cli.T(
			"比较当前配置与 --candidate（目录、主配置文件、版本号或操作 ID，例如 v2026.11.18.1、op_20261118_001）。只展示差异，不应用。\n"+
				"敏感键只显示 changed，不输出新旧值。无法解析时失败，不会用空差异冒充一致。\n"+
				"候选为目录或版本快照时比较整棵 JSON 树（主文件 + conf.d）。",
			"Compare the current config with --candidate (a directory, main file, version or operation id,\n"+
				"e.g. v2026.11.18.1 or op_20261118_001).\n"+
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
				return cli.UsageError(fmt.Errorf("--candidate is required"))
			}
			if err := rejectCandidateEscape(candidate); err != nil {
				return err
			}
			curPath, err := resolveDiffRef("current", current)
			if err != nil {
				return err
			}
			candPath, err := resolveDiffRef("candidate", candidate)
			if err != nil {
				return err
			}
			data, err := diffConfig(curPath, candPath)
			if err != nil {
				return cli.FromViewError("config.diff", err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "config.diff", data, nil)
			}
			if data.Identical {
				fmt.Fprintln(cmd.OutOrStdout(), "identical: true")
				return nil
			}
			return cli.PrintIndentedJSON(cmd.OutOrStdout(), data)
		},
	}
	cli.AddConfigFlag(c)
	c.Flags().String("current", "", cli.T("当前配置文件或目录，默认与 --config 相同", "current config file or directory, defaults to --config"))
	c.Flags().String("candidate", "", cli.T("候选配置目录、文件或版本号（必填）", "candidate config directory, file, or version (required)"))
	return c
}

func rejectCandidateEscape(path string) error {
	if _, err := config.ResolveCandidateRoot(path); err != nil {
		if errors.Is(err, config.ErrCandidatePath) {
			return cli.UsageError(err)
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
	return resolveDiffRef("candidate", candidate)
}

// resolveDiffRef 把路径、版本号（v2026.11.18.1）或操作 ID（op_...）解析成可比较的路径。
// --current 也接受版本号，方便直接对比两个历史版本。
func resolveDiffRef(kind, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ref, nil
	}
	if _, err := os.Stat(ref); err == nil {
		return ref, nil
	}
	if strings.HasPrefix(ref, "op_") {
		rec, err := operation.Store{Dir: cli.StateDir()}.Get(ref)
		if err != nil {
			return "", &cli.CLIError{Code: cli.CodeNotFound, Command: "config.diff", Message: kind + " not found: " + ref, Cause: err}
		}
		v, _ := rec.Result["version"].(string)
		if v == "" {
			return "", &cli.CLIError{Code: cli.CodeNotFound, Command: "config.diff", Message: "operation has no config version: " + ref}
		}
		ref = v
	}
	if cfgver.IsVersionName(ref) {
		dest := cfgver.VersionDir(cli.StateDir(), ref)
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			return dest, nil
		}
		p, err := cfgver.MainFile(cli.StateDir(), ref)
		if err != nil {
			return "", &cli.CLIError{Code: cli.CodeNotFound, Command: "config.diff", Message: kind + " not found: " + ref, Cause: err}
		}
		return p, nil
	}
	if kind == "current" {
		return ref, nil
	}
	return "", &cli.CLIError{Code: cli.CodeNotFound, Command: "config.diff", Message: kind + " not found: " + ref}
}

func newConfigApplyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "apply",
		Short: cli.T("将候选配置目录受控应用到线上", "apply a candidate config directory under management control"),
		Long: cli.T(
			"校验候选目录、生成差异、备份、切换配置、reload。读得到 /runtime 时做强验证，否则退回弱验证。失败则从临时备份还原。\n"+
				"脚本中必须加 --yes；在终端里会先询问 y/N。--dry-run 只输出校验与差异，不写正式文件、不 reload、不建快照。成功后自动保存结果快照。",
			"Validate the candidate directory, diff it, back up, switch files, reload, and verify.\n"+
				"When /runtime is reachable the check is strong; otherwise it falls back to a basic process/port check.\n"+
				"Failure restores from a temporary backup. Scripts must pass --yes; interactive terminals are asked y/N first.\n"+
				"--dry-run only reports validation and the diff; it writes nothing, does not reload, and creates no snapshot.\n"+
				"On success a result snapshot is saved automatically."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigApply,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("candidate", "", cli.T("候选配置目录（必填）", "candidate config directory (required)"))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不应用", "validate and show the diff only, do not apply"))
	c.Flags().Bool("yes", false, cli.T("确认执行（脚本中非 dry-run 必填；终端中会询问 y/N）", "confirm the apply (required in scripts; interactive terminals are asked y/N)"))
	return c
}

func newConfigRollbackCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rollback",
		Short: cli.T("回滚到一个历史配置版本", "roll back to a historical config version"),
		Long: cli.T(
			"回滚复用目标版本的快照（主配置、conf.d 与服务证书），再走与 apply 相同的事务，不会新建版本。\n"+
				"目标可以是版本号（v2026.11.18.1）或操作 ID（op_...），二选一。\n"+
				"回滚会在操作记录里留一条 config.rollback，方便审计，但版本列表不把它当成新版本。脚本中必须加 --yes；在终端里会先询问 y/N。",
			"Rollback reuses the target version snapshot (main config, conf.d, and service certificates)\n"+
				"and runs the same transaction as apply. It never creates a new version.\n"+
				"The target is either a version (v2026.11.18.1) or an operation id (op_...).\n"+
				"Rollback leaves a config.rollback entry in the operation log for auditing, but the version list\n"+
				"does not treat it as a new version. Scripts must pass --yes; interactive terminals are asked y/N first."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigRollback,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("version", "", cli.T("目标版本号，例如 v2026.11.18.1（与 --operation 二选一）", "target version, e.g. v2026.11.18.1 (either this or --operation)"))
	c.Flags().String("operation", "", cli.T("目标操作 ID，例如 op_20261118_001（与 --version 二选一）", "target operation id, e.g. op_20261118_001 (either this or --version)"))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不回滚", "validate and show the diff only, do not roll back"))
	c.Flags().Bool("yes", false, cli.T("确认执行（脚本中非 dry-run 必填；终端中会询问 y/N）", "confirm the rollback (required in scripts; interactive terminals are asked y/N)"))
	return c
}
func runConfigApply(cmd *cobra.Command, _ []string) error {
	cand, _ := cmd.Flags().GetString("candidate")
	if strings.TrimSpace(cand) == "" {
		return cli.UsageError(fmt.Errorf("--candidate is required"))
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	if !dry {
		if err := cli.ConfirmWrite(cmd, cli.T("应用配置", "apply config")); err != nil {
			return err
		}
	}
	live, _ := cmd.Flags().GetString("config")
	return runTxn(cmd, "config.apply", txn.Input{
		Candidate: cand,
		LiveMain:  live,
		RequestID: cli.EnsureRequestID(cmd),
		Command:   "config.apply",
		Summary:   "config apply",
		DryRun:    dry,
	})
}

func runConfigRollback(cmd *cobra.Command, _ []string) error {
	ver, _ := cmd.Flags().GetString("version")
	opID, _ := cmd.Flags().GetString("operation")
	ver, opID = strings.TrimSpace(ver), strings.TrimSpace(opID)
	if (ver == "") == (opID == "") {
		return cli.UsageError(fmt.Errorf("exactly one of --version or --operation is required"))
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	if !dry {
		target := ver
		if target == "" {
			target = opID
		}
		if err := cli.ConfirmWrite(cmd, cli.T("回滚配置到 "+target, "roll back config to "+target)); err != nil {
			return err
		}
	}
	if opID != "" {
		rec, err := operation.Store{Dir: cli.StateDir()}.Get(opID)
		if err != nil {
			return &cli.CLIError{Code: cli.CodeNotFound, Command: "config.rollback", Message: "operation not found: " + opID, Cause: err}
		}
		v, _ := rec.Result["version"].(string)
		if v == "" {
			return &cli.CLIError{Code: cli.CodeNotFound, Command: "config.rollback", Message: "operation has no config version: " + opID}
		}
		ver = v
	}
	root := cfgver.VersionDir(cli.StateDir(), ver)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return &cli.CLIError{Code: cli.CodeNotFound, Command: "config.rollback", Message: "version not found: " + ver}
	}
	live, _ := cmd.Flags().GetString("config")
	args := map[string]string{"version": ver, "live": live}
	if opID != "" {
		args["operation"] = opID
	}
	return runTxn(cmd, "config.rollback", txn.Input{
		Candidate:    root,
		LiveMain:     live,
		RequestID:    cli.EnsureRequestID(cmd),
		Command:      "config.rollback",
		Summary:      "rollback to " + ver,
		DryRun:       dry,
		RestoreCerts: true,
		Args:         args,
	})
}

func runTxn(cmd *cobra.Command, id string, in txn.Input) error {
	if err := rejectCandidateEscape(in.Candidate); err != nil {
		return err
	}
	timeout := cli.CmdTimeout(cmd, svcctl.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	eng := cli.NewTxnEngine(in.LiveMain, timeout)
	res, err := eng.Apply(ctx, in)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return cli.FromTxnError(id, &txn.Error{Code: txn.CodeUnknown, Message: err.Error(), Err: err, Operation: opPtr(res.Operation)})
		}
		return cli.FromTxnError(id, err)
	}
	return cli.EmitTxnResult(cmd, id, res)
}

func opPtr(rec operation.Record) *operation.Record {
	if rec.ID == "" {
		return nil
	}
	cp := rec
	return &cp
}
