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
				"站点名由 --domain 规范化得到。同名或同 server_name 已存在时退出码 10。非 dry-run 必须 --yes。",
			"Copy the live config tree, write the site file on the candidate, then apply it with the same transaction\n"+
				"as config apply. The site name is derived from --domain. A duplicate name or server_name exits with code 10.\n"+
				"--yes is required unless --dry-run."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runSiteAdd,
	}
	add.Flags().String("domain", "", t("站点域名（必填，同时用作 server_name 与文件名）", "site domain (required; used as server_name and file name)"))
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
		c.Flags().Bool("yes", false, t("确认执行（非 dry-run 必填）", "confirm (required unless --dry-run)"))
		c.Flags().Bool("confirm", false, t("与 --yes 同义", "same as --yes"))
		root.AddCommand(c)
	}
}

func siteWriteYes(cmd *cobra.Command) error {
	dry, _ := cmd.Flags().GetBool("dry-run")
	if dry || confirmed(cmd) {
		return nil
	}
	return usageError(fmt.Errorf("%s requires --yes (or --confirm); use --dry-run to preview", cmd.Name()))
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
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
	domain, _ := cmd.Flags().GetString("domain")
	if strings.TrimSpace(domain) == "" {
		return usageError(fmt.Errorf("--domain is required"))
	}
	live, _ := cmd.Flags().GetString("config")
	root, _ := cmd.Flags().GetString("root")
	proxy, _ := cmd.Flags().GetString("proxy-pass")
	if strings.TrimSpace(proxy) == "" && strings.TrimSpace(root) == "" {
		return usageError(fmt.Errorf("--root or --proxy-pass is required"))
	}
	listens, _ := cmd.Flags().GetStringArray("listen")
	tls, _ := cmd.Flags().GetBool("tls")
	dry, _ := cmd.Flags().GetBool("dry-run")
	in := siteops.AddInput{Domain: domain, Root: root, Listens: listens, ProxyPass: proxy, TLS: tls, Banned: bannedRoots(live)}
	return runSiteTxn(cmd, "site.add", "site add "+domain, map[string]string{"domain": domain}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Add(dirs, in)
		return err
	})
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
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
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
