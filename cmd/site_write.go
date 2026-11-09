package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"fast-https/config"
	"fast-https/management/pathguard"
	"fast-https/management/siteops"
	"fast-https/management/svcctl"
	"fast-https/management/view"

	"github.com/spf13/cobra"
)

func addSiteWriteCommands(root *cobra.Command) {
	add := &cobra.Command{
		Use:   "add",
		Short: t("受控新增站点（走配置事务）", "add a site through the config transaction"),
		Long: t(
			"复制当前配置树到候选目录、写入站点文件，再走 config apply 同一套事务（校验、恢复点、reload、可回滚）。\n"+
				"带 --domain 时全部字段来自 flag；不带 --domain 且标准输入是终端时进向导。\n"+
				"站点名由域名规范化得到。同名或同 server_name 已存在时退出码 10。纯 flag 模式非 dry-run 必须 --yes；向导模式在最后一屏确认。",
			"Copy the live config tree, write the site file on the candidate, then apply it with the same transaction\n"+
				"as config apply. With --domain everything comes from flags; without it, an interactive terminal enters the wizard.\n"+
				"A duplicate name or server_name exits with code 10. Flag mode requires --yes unless --dry-run; the wizard confirms at the end."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runSiteAdd,
	}
	add.Flags().String("domain", "", t("站点域名（同时用作 server_name 与文件名；省略且能交互时进向导）", "site domain (used as server_name and file name; omit to enter the wizard when interactive)"))
	add.Flags().String("root", "", t("本地站点文档根目录", "document root for a local site"))
	add.Flags().StringArray("listen", nil, t("监听值，可重复", "listen value, repeatable"))
	add.Flags().String("proxy-pass", "", t("反代上游，设置后站点类型为 proxy", "proxy upstream; when set, the site type is proxy"))
	add.Flags().Bool("tls", false, t("启用 TLS", "enable TLS"))

	upd := &cobra.Command{
		Use:   "update <name>",
		Short: t("受控更新站点的白名单字段", "update allow-listed fields of a site through the config transaction"),
		Long: t(
			"只允许 --listen、--server-name、--root、--proxy-pass、--tls。不接受任意 JSON 合并。内联在主配置 http.server[] 里的站点返回 NOT_SUPPORTED。",
			"Only --listen, --server-name, --root, --proxy-pass and --tls are accepted. Arbitrary JSON merge is rejected.\n"+
				"Sites inline in the main config http.server[] return NOT_SUPPORTED."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runSiteUpdate,
	}
	upd.Flags().StringArray("listen", nil, t("新的监听值", "new listen values"))
	upd.Flags().String("server-name", "", t("新的 server_name", "new server_name"))
	upd.Flags().String("root", "", t("新的文档根目录", "new document root"))
	upd.Flags().String("proxy-pass", "", t("新的反代上游", "new proxy upstream"))
	upd.Flags().Bool("tls", false, t("启用 TLS", "enable TLS"))

	del := &cobra.Command{
		Use:          "delete <name>",
		Short:        t("受控删除站点配置（不删内容目录与证书）", "delete a site config without removing content or certificates"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runSiteDelete,
	}

	dis := &cobra.Command{
		Use:          "disable <name>",
		Short:        t("停用站点（改名为 .json.disabled 并 reload）", "disable a site (rename to .json.disabled and reload)"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runSiteDisable,
	}
	en := &cobra.Command{
		Use:          "enable <name>",
		Short:        t("启用已停用的站点并 reload", "enable a disabled site and reload"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runSiteEnable,
	}

	for _, c := range []*cobra.Command{add, upd, del, dis, en} {
		addConfigFlag(c)
		c.Flags().Bool("dry-run", false, t("只生成差异，不应用", "show the diff only, do not apply"))
		c.Flags().Bool("yes", false, t("确认执行（add/update/delete 非 dry-run 必填；disable/enable 不需要）",
			"confirm (required for add/update/delete unless --dry-run; not needed for disable/enable)"))
		root.AddCommand(c)
	}
}

func siteWriteYes(cmd *cobra.Command) error {
	dry, _ := cmd.Flags().GetBool("dry-run")
	if dry || confirmed(cmd) {
		return nil
	}
	return usageError(fmt.Errorf("%s requires --yes; use --dry-run to preview", cmd.Name()))
}

func bannedRoots(liveMain string) []string {
	return []string{
		filepath.Dir(liveMain),
		config.CERT_DIR,
		resolveStateDir(),
		config.DEFAULT_LOG_ROOT,
	}
}

func runSiteAdd(cmd *cobra.Command, _ []string) error {
	live, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	// 没给 --domain 时进向导：向导最后一屏本身就带确认，再要求 --yes
	// 会让用户还没看到任何问题就被拦住，等于确认两次。纯 flag 模式
	// 仍要显式确认，避免脚本误写。
	if cmd.Flags().Changed("domain") {
		if err := siteWriteYes(cmd); err != nil {
			return err
		}
	}
	in, err := collectAddInput(cmd, bannedRoots(live))
	if err != nil {
		return err
	}
	return runSiteTxn(cmd, "site.add", "site add "+in.Domain, map[string]string{"domain": in.Domain}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Add(dirs, in)
		return err
	})
}

// collectAddInput 采集新增站点需要的字段：带 --domain 走 flag（非交互，
// 供脚本与 agent 调用）；不带 --domain 且能交互时进向导，把向导结果压成
// 同一个 AddInput，两条路径最终都落到 siteops.Add 的配置事务上。
func collectAddInput(cmd *cobra.Command, banned []string) (siteops.AddInput, error) {
	domain, _ := cmd.Flags().GetString("domain")
	root, _ := cmd.Flags().GetString("root")
	proxy, _ := cmd.Flags().GetString("proxy-pass")
	listens, _ := cmd.Flags().GetStringArray("listen")
	tls, _ := cmd.Flags().GetBool("tls")

	if strings.TrimSpace(domain) != "" {
		if strings.TrimSpace(proxy) == "" && strings.TrimSpace(root) == "" {
			return siteops.AddInput{}, usageError(fmt.Errorf("--root or --proxy-pass is required"))
		}
		// root 的边界在事务外先校验：路径越界属于使用错误，没必要先建
		// 恢复点再回滚，否则失败会留下 uncertain 状态，后续命令被
		// RECOVERY_REQUIRED 拦住。
		if strings.TrimSpace(proxy) == "" {
			abs, err := pathguard.CheckRoot(root, banned)
			if err != nil {
				return siteops.AddInput{}, usageError(err)
			}
			root = abs
		}
		return siteops.AddInput{Domain: domain, Root: root, Listens: listens, ProxyPass: proxy, TLS: tls, Banned: banned}, nil
	}

	// 没给 --domain：能交互就进向导（原 site init 的能力）。
	opts := siteOptionsFrom(cmd, nil)
	name, si, err := siteInputFrom(&opts)
	if err != nil {
		return siteops.AddInput{}, usageError(
			fmt.Errorf("未提供 --domain，且当前无法进入向导（标准输入不是终端）：%w", err))
	}
	var loc config.SiteLocation
	if len(si.Locations) > 0 {
		loc = si.Locations[0]
	}
	// 站点文件名取主机名：add 的语义是「域名即文件名」，带空格的多域名
	// 只取第一个，其余用 site update --server-name 补。
	primary := strings.TrimSpace(name)
	if fields := strings.Fields(si.ServerName); len(fields) > 0 {
		primary = fields[0]
	}
	if primary == "" {
		return siteops.AddInput{}, usageError(fmt.Errorf("--domain is required"))
	}
	return siteops.AddInput{
		Domain:    primary,
		Root:      loc.Root,
		ProxyPass: loc.ProxyPass,
		Listens:   si.Listens,
		TLS:       listensHaveSSL(si.Listens),
		Banned:    banned,
	}, nil
}

func runSiteUpdate(cmd *cobra.Command, args []string) error {
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
	live, _ := cmd.Flags().GetString("config")
	in := siteops.UpdateInput{Name: strings.TrimSuffix(args[0], ".json"), Banned: bannedRoots(live)}
	changed := false
	if cmd.Flags().Changed("listen") {
		v, _ := cmd.Flags().GetStringArray("listen")
		in.Listens = &v
		changed = true
	}
	if cmd.Flags().Changed("server-name") {
		v, _ := cmd.Flags().GetString("server-name")
		in.ServerName = &v
		changed = true
	}
	if cmd.Flags().Changed("root") {
		v, _ := cmd.Flags().GetString("root")
		in.Root = &v
		changed = true
	}
	if cmd.Flags().Changed("proxy-pass") {
		v, _ := cmd.Flags().GetString("proxy-pass")
		in.ProxyPass = &v
		changed = true
	}
	if cmd.Flags().Changed("tls") {
		v, _ := cmd.Flags().GetBool("tls")
		in.TLS = &v
		changed = true
	}
	if !changed {
		return usageError(fmt.Errorf("update requires at least one of --listen, --server-name, --root, --proxy-pass, --tls"))
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	return runSiteTxn(cmd, "site.update", "site update "+in.Name, map[string]string{"name": in.Name}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Update(dirs, live, in)
		return err
	})
}

func runSiteDelete(cmd *cobra.Command, args []string) error {
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
	name := strings.TrimSuffix(args[0], ".json")
	live, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	return runSiteTxn(cmd, "site.delete", "site delete "+name, map[string]string{"name": name}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Delete(dirs, live, name)
		return err
	})
}

func runSiteDisable(cmd *cobra.Command, args []string) error {
	return runSiteEnablement(cmd, args, false)
}

func runSiteEnable(cmd *cobra.Command, args []string) error {
	return runSiteEnablement(cmd, args, true)
}

func runSiteEnablement(cmd *cobra.Command, args []string, enabled bool) error {
	// 启用/停用只是把名字在 .json 与 .json.disabled 之间改，随时能改回来，
	// 不像 delete 那样不可逆，所以不再要求 --yes。
	name := strings.TrimSuffix(args[0], ".json")
	live, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	id, summary := "site.disable", "site disable "+name
	if enabled {
		id, summary = "site.enable", "site enable "+name
	}
	return runSiteTxn(cmd, id, summary, map[string]string{"name": name, "enabled": fmt.Sprintf("%v", enabled)}, dry, func(_ string, dirs []string) error {
		_, err := siteops.SetEnabled(dirs, live, name, enabled)
		return err
	})
}

func runSiteTxn(cmd *cobra.Command, id, summary string, args map[string]string, dry bool, mut func(string, []string) error) error {
	live, _ := cmd.Flags().GetString("config")
	timeout := cmdTimeout(cmd, svcctl.DefaultTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	eng := newTxnEngine(live, timeout)
	res, err := siteops.Run(ctx, eng, siteops.RunInput{
		LiveMain:  live,
		RequestID: ensureRequestID(cmd),
		Command:   id,
		Summary:   summary,
		DryRun:    dry,
		Args:      args,
		Mutate:    mut,
	})
	if err != nil {
		return fromSiteopsError(id, err)
	}
	return emitTxnResult(cmd, id, res)
}

func fromSiteopsError(id string, err error) *CLIError {
	var inv *view.InvalidError
	switch {
	case errors.Is(err, siteops.ErrUsage), errors.Is(err, pathguard.ErrPath):
		return &CLIError{Code: CodeUsage, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrConflict):
		return &CLIError{Code: CodeConflict, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrNotFound):
		return &CLIError{Code: CodeNotFound, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, siteops.ErrNotSupported):
		return &CLIError{Code: CodeNotSupported, Command: id, Message: err.Error(), Cause: err}
	case errors.As(err, &inv):
		return &CLIError{Code: CodeConfigInvalid, Command: id, Message: err.Error(), Cause: err}
	default:
		return fromTxnError(id, err)
	}
}
