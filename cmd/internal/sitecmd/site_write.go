package sitecmd

import (
	"fast-https/cmd/internal/cli"
	"fmt"
	"strings"

	"fast-https/config"
	"fast-https/management/pathguard"
	"fast-https/management/siteops"

	"github.com/spf13/cobra"
)

func addSiteWriteCommands(root *cobra.Command) {
	add := &cobra.Command{
		Use:   "add",
		Short: cli.T("受控新增站点（走配置事务）", "add a site through the config transaction"),
		Long: cli.T(
			"复制当前配置树到候选目录、写入站点文件，再走 config apply 同一套事务（校验、恢复点、reload、可回滚）。\n"+
				"带 --domain 时全部字段来自 flag；不带 --domain 且标准输入是终端时进向导。\n"+
				"站点名由域名规范化得到。同名或同 server_name 已存在时退出码 10。纯 flag 模式非 dry-run 必须 --yes；向导模式在最后一屏确认。",
			"Copy the live config tree, write the site file on the candidate, then apply it with the same transaction\n"+
				"as config apply. With --domain everything comes from flags; without it, an interactive terminal enters the wizard.\n"+
				"A duplicate name or server_name exits with code 10. Flag mode requires --yes unless --dry-run; the wizard confirms at the end."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         RunAdd,
	}
	add.Flags().String("domain", "", cli.T("站点域名（同时用作 server_name 与文件名；省略且能交互时进向导）", "site domain (used as server_name and file name; omit to enter the wizard when interactive)"))
	add.Flags().String("root", "", cli.T("本地站点文档根目录", "document root for a local site"))
	add.Flags().StringArray("listen", nil, cli.T("监听值，可重复", "listen value, repeatable"))
	add.Flags().String("proxy-pass", "", cli.T("反代上游，设置后站点类型为 proxy", "proxy upstream; when set, the site type is proxy"))
	add.Flags().Bool("tls", false, cli.T("启用 TLS", "enable TLS"))

	upd := &cobra.Command{
		Use:   "update <name>",
		Short: cli.T("受控更新站点的白名单字段", "update allow-listed fields of a site through the config transaction"),
		Long: cli.T(
			"只改 flag 点名的字段：--listen、--server-name、--root、--proxy-pass、--tls，"+
				"以及主 location 上的高级配置 --zip、--limit-*、--auth-*、--cache-*。"+
				"没写的字段（含未知键与注释）原样保留，不接受任意 JSON 合并。"+
				"关闭某项：--zip= 清空压缩，--auth-type= 关闭认证，--cache-path= 关闭缓存。"+
				"--auth-pass 留空或传 *** 表示沿用原口令。内联在主配置 http.server[] 里的站点返回 NOT_SUPPORTED。",
			"Only the named flags change: --listen, --server-name, --root, --proxy-pass, --tls,\n"+
				"plus main-location advanced options --zip, --limit-*, --auth-*, --cache-*.\n"+
				"Everything else, including unknown keys and comments, is preserved; arbitrary JSON merge is rejected.\n"+
				"Disable with --zip=, --auth-type= or --cache-path=. An empty or *** --auth-pass keeps the current password.\n"+
				"Sites inline in the main config http.server[] return NOT_SUPPORTED."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         RunUpdate,
	}
	upd.Flags().StringArray("listen", nil, cli.T("新的监听值", "new listen values"))
	upd.Flags().String("server-name", "", cli.T("新的 server_name", "new server_name"))
	upd.Flags().String("root", "", cli.T("新的文档根目录", "new document root"))
	upd.Flags().String("proxy-pass", "", cli.T("新的反代上游", "new proxy upstream"))
	upd.Flags().Bool("tls", false, cli.T("启用 TLS", "enable TLS"))
	upd.Flags().StringSlice("zip", nil, cli.T("响应压缩编码，可重复：gzip / br；传空表示关闭", "response compression: gzip / br, repeatable; empty disables"))
	upd.Flags().Int("limit-rate", -1, cli.T("限流 rate（每秒请求数），0 表示不限", "rate limit (requests per second); 0 disables"))
	upd.Flags().Int("limit-burst", -1, cli.T("限流 burst（突发桶大小）", "rate limit burst bucket size"))
	upd.Flags().Int("limit-mem", -1, cli.T("限流 mem（连接缓冲大小）", "rate limit connection buffer size"))
	upd.Flags().Bool("limit-nodelay", false, cli.T("限流 nodelay：不排队直接拒绝", "rate limit nodelay: reject instead of queueing"))
	upd.Flags().String("auth-type", "", cli.T("认证类型，目前只支持 basic；留空表示关闭认证", "auth type, only basic is supported; empty disables auth"))
	upd.Flags().String("auth-user", "", cli.T("Basic 认证用户名", "basic auth user name"))
	upd.Flags().String("auth-pass", "", cli.T("Basic 认证口令；留空或 *** 表示沿用原口令", "basic auth password; empty or *** keeps the current one"))
	upd.Flags().String("auth-realm", "", cli.T("Basic 认证 realm，留空用站点名", "basic auth realm; defaults to the site name"))
	upd.Flags().String("cache-path", "", cli.T("代理缓存目录；留空表示关闭缓存", "proxy cache directory; empty disables caching"))
	upd.Flags().StringSlice("cache-valid", nil, cli.T("代理缓存有效期，可重复，如 10s / 1m", "proxy cache TTL values, repeatable, e.g. 10s / 1m"))
	upd.Flags().Int("cache-max-size", -1, cli.T("代理缓存最大字节数", "proxy cache maximum size in bytes"))

	del := &cobra.Command{
		Use:          "delete <name>",
		Short:        cli.T("受控删除站点配置（不删内容目录与证书）", "delete a site config without removing content or certificates"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         RunDelete,
	}

	dis := &cobra.Command{
		Use:          "disable <name>",
		Short:        cli.T("停用站点（改名为 .json.disabled 并 reload）", "disable a site (rename to .json.disabled and reload)"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         RunDisable,
	}
	en := &cobra.Command{
		Use:          "enable <name>",
		Short:        cli.T("启用已停用的站点并 reload", "enable a disabled site and reload"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         RunEnable,
	}

	for _, c := range []*cobra.Command{add, upd, del, dis, en} {
		cli.AddConfigFlag(c)
		c.Flags().Bool("dry-run", false, cli.T("只生成差异，不应用", "show the diff only, do not apply"))
		c.Flags().Bool("yes", false, cli.T("确认执行（add/update/delete 非 dry-run 必填；disable/enable 不需要）",
			"confirm (required for add/update/delete unless --dry-run; not needed for disable/enable)"))
		root.AddCommand(c)
	}
}

func siteWriteYes(cmd *cobra.Command) error {
	dry, _ := cmd.Flags().GetBool("dry-run")
	if dry || cli.Confirmed(cmd) {
		return nil
	}
	return cli.UsageError(fmt.Errorf("%s requires --yes; use --dry-run to preview", cmd.Name()))
}
func RunAdd(cmd *cobra.Command, _ []string) error {
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
	in, err := collectAddInput(cmd, cli.BannedRoots(live))
	if err != nil {
		return err
	}
	return cli.RunTxn(cmd, "site.add", "site add "+in.Domain, map[string]string{"domain": in.Domain}, dry, func(_ string, dirs []string) error {
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
			return siteops.AddInput{}, cli.UsageError(fmt.Errorf("--root or --proxy-pass is required"))
		}
		// root 的边界在事务外先校验：路径越界属于使用错误，没必要先建
		// 恢复点再回滚，否则失败会留下 uncertain 状态，后续命令被
		// RECOVERY_REQUIRED 拦住。
		if strings.TrimSpace(proxy) == "" {
			abs, err := pathguard.CheckRoot(root, banned)
			if err != nil {
				return siteops.AddInput{}, cli.UsageError(err)
			}
			root = abs
		}
		return siteops.AddInput{Domain: domain, Root: root, Listens: listens, ProxyPass: proxy, TLS: tls, Banned: banned}, nil
	}

	// 没给 --domain：能交互就进向导（原 site init 的能力）。
	opts := OptionsFrom(cmd, nil)
	name, si, err := siteInputFrom(&opts)
	if err != nil {
		return siteops.AddInput{}, cli.UsageError(
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
		return siteops.AddInput{}, cli.UsageError(fmt.Errorf("--domain is required"))
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

func RunUpdate(cmd *cobra.Command, args []string) error {
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
	live, _ := cmd.Flags().GetString("config")
	in := siteops.UpdateInput{Name: strings.TrimSuffix(args[0], ".json"), Banned: cli.BannedRoots(live)}
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
	if cmd.Flags().Changed("zip") {
		v, _ := cmd.Flags().GetStringSlice("zip")
		if len(v) == 1 && strings.TrimSpace(v[0]) == "" {
			v = nil // --zip "" 表示显式关闭压缩
		}
		in.Zip = &v
		changed = true
	}
	if limit, ok := readLimitFlags(cmd); ok {
		in.Limit = limit
		changed = true
	}
	if auth, ok := readAuthFlags(cmd); ok {
		in.Auth = auth
		changed = true
	}
	if cache, ok := readCacheFlags(cmd); ok {
		in.Cache = cache
		changed = true
	}
	if !changed {
		return cli.UsageError(fmt.Errorf("update requires at least one of --listen, --server-name, --root, --proxy-pass, --tls, --zip, --limit-*, --auth-*, --cache-*"))
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	return cli.RunTxn(cmd, "site.update", "site update "+in.Name, map[string]string{"name": in.Name}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Update(dirs, live, in)
		return err
	})
}

func RunDelete(cmd *cobra.Command, args []string) error {
	if err := siteWriteYes(cmd); err != nil {
		return err
	}
	name := strings.TrimSuffix(args[0], ".json")
	live, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	return cli.RunTxn(cmd, "site.delete", "site delete "+name, map[string]string{"name": name}, dry, func(_ string, dirs []string) error {
		_, err := siteops.Delete(dirs, live, name)
		return err
	})
}

func RunDisable(cmd *cobra.Command, args []string) error {
	return RunEnablement(cmd, args, false)
}

// readLimitFlags 组装 --limit-*：任一个被显式指定就参与写入。
// 未指定的项保持 0，与「清除限流」等价——CLI 无法区分"没写"和"写 0"，
// 所以只要用户点了任一 limit flag，就当作整块 limit 的新值。
func readLimitFlags(cmd *cobra.Command) (*config.PathLimit, bool) {
	f := cmd.Flags()
	if !f.Changed("limit-rate") && !f.Changed("limit-burst") &&
		!f.Changed("limit-mem") && !f.Changed("limit-nodelay") {
		return nil, false
	}
	limit := &config.PathLimit{}
	if f.Changed("limit-rate") {
		limit.Rate, _ = f.GetInt("limit-rate")
	}
	if f.Changed("limit-burst") {
		limit.Burst, _ = f.GetInt("limit-burst")
	}
	if f.Changed("limit-mem") {
		limit.Size, _ = f.GetInt("limit-mem")
	}
	if f.Changed("limit-nodelay") {
		limit.Nodelay, _ = f.GetBool("limit-nodelay")
	}
	return limit, true
}

// readAuthFlags 组装 --auth-*。四个 flag 全空等于关闭认证（补丁删掉 auth 键）。
func readAuthFlags(cmd *cobra.Command) (*config.PathAuth, bool) {
	f := cmd.Flags()
	if !f.Changed("auth-type") && !f.Changed("auth-user") &&
		!f.Changed("auth-pass") && !f.Changed("auth-realm") {
		return nil, false
	}
	auth := &config.PathAuth{}
	auth.AuthType, _ = f.GetString("auth-type")
	auth.User, _ = f.GetString("auth-user")
	auth.Pswd, _ = f.GetString("auth-pass")
	auth.Realm, _ = f.GetString("auth-realm")
	return auth, true
}

// readCacheFlags 组装 --cache-*。path 为空等于关闭缓存。
func readCacheFlags(cmd *cobra.Command) (*config.Cache, bool) {
	f := cmd.Flags()
	if !f.Changed("cache-path") && !f.Changed("cache-valid") && !f.Changed("cache-max-size") {
		return nil, false
	}
	cache := &config.Cache{}
	cache.Path, _ = f.GetString("cache-path")
	cache.Valid, _ = f.GetStringSlice("cache-valid")
	if f.Changed("cache-max-size") {
		cache.MaxSize, _ = f.GetInt("cache-max-size")
	}
	return cache, true
}

func RunEnable(cmd *cobra.Command, args []string) error {
	return RunEnablement(cmd, args, true)
}

func RunEnablement(cmd *cobra.Command, args []string, enabled bool) error {
	// 启用/停用只是把名字在 .json 与 .json.disabled 之间改，随时能改回来，
	// 不像 delete 那样不可逆，所以不再要求 --yes。
	name := strings.TrimSuffix(args[0], ".json")
	live, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	id, summary := "site.disable", "site disable "+name
	if enabled {
		id, summary = "site.enable", "site enable "+name
	}
	return cli.RunTxn(cmd, id, summary, map[string]string{"name": name, "enabled": fmt.Sprintf("%v", enabled)}, dry, func(_ string, dirs []string) error {
		_, err := siteops.SetEnabled(dirs, live, name, enabled)
		return err
	})
}
