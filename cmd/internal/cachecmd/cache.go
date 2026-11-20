package cachecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fast-https/cmd/internal/cli"
	"fast-https/config"

	"github.com/spf13/cobra"
)

// cache 命令组管理主配置顶层的 cache 段：缓存区在这里定义目录、容量与
// 有效期，站点文件只写 proxy_cache: "<区名>" 引用。
//
// 与 config proxy / config tls 同一套路：文本级局部替换，注释与其它字段
// 原样保留；写入前按主配置完整校验一遍。写入后不自动 reload。
func NewCacheCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cache",
		Short: cli.T("管理主配置里的缓存区（目录、容量、有效期）", "manage cache zones (directory, size, TTL) in the main config"),
		Long: cli.T(
			"缓存区定义在主配置的 cache 段，站点 location 用 proxy_cache 引用区名。\n"+
				"目录相对程序数据目录（Windows 归档为 exe 所在目录，Linux 包为 /usr/share/fast-https）。\n"+
				"缓存键自动生成（$host$request_uri，带区名前缀），方法固定 GET / HEAD。\n"+
				"写入后不自动 reload：下次 reload 或启动时生效。",
			"Cache zones live in the main config cache section; a site location references one by name via proxy_cache.\n"+
				"Directories are relative to the program data directory (next to the exe in the Windows archive,\n"+
				"/usr/share/fast-https in Linux packages). Cache keys are generated ($host$request_uri with a zone prefix)\n"+
				"and only GET / HEAD are cached. Changes take effect on the next reload or restart."),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	root.AddCommand(newListCmd(), newAddCmd(), newUpdateCmd(), newDeleteCmd())
	return root
}

func newListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "list",
		Short:        cli.T("列出缓存区", "list cache zones"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runList,
	}
	cli.AddConfigFlag(c)
	return c
}

func newAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add",
		Short: cli.T("新增缓存区", "add a cache zone"),
		Long: cli.T(
			"新增一个缓存区。名称必填且唯一，目录必须写在明确的位置（相对数据目录）。\n"+
				"--valid 可重复，写法为 状态码:时长 或 any:时长，例如 200:10m、404:1m、any:30s。\n"+
				"--max-size 为 0 表示不限制容量。脚本中非 dry-run 必须加 --yes；在终端里会先询问 y/N。",
			"Add a cache zone. The name is required and unique, and the directory must be explicit (relative to the data directory).\n"+
				"--valid is repeatable as status:duration or any:duration, e.g. 200:10m, 404:1m, any:30s.\n"+
				"--max-size 0 means unlimited. Unless --dry-run, scripts must pass --yes; interactive terminals are asked y/N first."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runAdd,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("name", "", cli.T("缓存区名（站点 proxy_cache 引用它）", "cache zone name (referenced by site proxy_cache)"))
	c.Flags().String("path", "", cli.T("缓存目录，相对程序数据目录", "cache directory, relative to the program data directory"))
	c.Flags().StringSlice("valid", nil, cli.T("有效期，可重复：200:10m / any:1h", "TTLs, repeatable: 200:10m / any:1h"))
	c.Flags().Int("max-size", 0, cli.T("最大字节数，0 表示不限制", "maximum size in bytes, 0 means unlimited"))
	c.Flags().Bool("dry-run", false, cli.T("只打印改写后的配置，不写盘", "print the patched config instead of writing it"))
	c.Flags().Bool("yes", false, cli.T("确认新增（非 dry-run 时，脚本必须显式传入；终端中会询问 y/N）", "confirm the add (required in scripts; interactive terminals are asked y/N)"))
	return c
}

func newUpdateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "update <name>",
		Short: cli.T("修改缓存区的目录、有效期或容量", "update a cache zone directory, TTLs or size"),
		Long: cli.T(
			"只改 flag 点名的字段：--path、--valid、--max-size。未写的字段保持不变。\n"+
				"改名请用 delete + add：站点文件按名字引用，改名会断开引用。\n"+
				"脚本中非 dry-run 必须加 --yes；在终端里会先询问 y/N。",
			"Only the named flags change: --path, --valid, --max-size. Other fields stay as they are.\n"+
				"Renaming is delete + add: sites reference zones by name, so a rename would break them.\n"+
				"Unless --dry-run, scripts must pass --yes; interactive terminals are asked y/N first."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runUpdate,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("path", "", cli.T("新的缓存目录", "new cache directory"))
	c.Flags().StringSlice("valid", nil, cli.T("新的有效期，可重复：200:10m / any:1h", "new TTLs, repeatable: 200:10m / any:1h"))
	c.Flags().Int("max-size", -1, cli.T("新的最大字节数，0 表示不限制", "new maximum size in bytes, 0 means unlimited"))
	c.Flags().Bool("dry-run", false, cli.T("只打印改写后的配置，不写盘", "print the patched config instead of writing it"))
	c.Flags().Bool("yes", false, cli.T("确认修改（非 dry-run 时，脚本必须显式传入；终端中会询问 y/N）", "confirm the update (required in scripts; interactive terminals are asked y/N)"))
	return c
}

func newDeleteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "delete <name>",
		Short: cli.T("删除缓存区", "delete a cache zone"),
		Long: cli.T(
			"删除一个缓存区。仍被站点 location 引用时拒绝删除：先把站点改成别的区或关闭缓存。\n"+
				"默认只删配置，磁盘上的缓存文件保留；加 --purge 才会连同该区目录一起删除。\n"+
				"脚本中非 dry-run 必须加 --yes；在终端里会先询问 y/N。",
			"Delete a cache zone. It is refused while any site location still references it: point the site at another\n"+
				"zone or disable caching first. By default only the config entry is removed and cache files stay on disk;\n"+
				"--purge also removes the zone directory.\n"+
				"Unless --dry-run, scripts must pass --yes; interactive terminals are asked y/N first."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runDelete,
	}
	cli.AddConfigFlag(c)
	c.Flags().Bool("dry-run", false, cli.T("只打印改写后的配置，不写盘", "print the patched config instead of writing it"))
	c.Flags().Bool("purge", false, cli.T("同时删除该缓存区的磁盘目录", "also delete the zone directory on disk"))
	c.Flags().Bool("yes", false, cli.T("确认删除（非 dry-run 时，脚本必须显式传入；终端中会询问 y/N）", "confirm the delete (required in scripts; interactive terminals are asked y/N)"))
	return c
}

func runList(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	zones, err := config.ReadCacheZones(path)
	if err != nil {
		return configInvalid("cache.list", err)
	}
	if zones == nil {
		zones = []config.CacheZone{}
	}
	sort.SliceStable(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	if cli.JSONMode(cmd) {
		return cli.EmitOK(cmd, "cache.list", map[string]any{"zones": zones, "config": path}, nil)
	}
	if len(zones) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s 里没有缓存区。用 fast-https cache add 新建。\n", path)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-28s %-24s %s\n", "NAME", "PATH", "VALID", "MAX_SIZE")
	for _, z := range zones {
		valid := strings.Join(z.Valid, ", ")
		if valid == "" {
			valid = "(none)"
		}
		maxSize := "unlimited"
		if z.MaxSize > 0 {
			maxSize = fmt.Sprintf("%d", z.MaxSize)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-28s %-24s %s\n", z.Name, z.Path, valid, maxSize)
	}
	return nil
}

func runAdd(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	name, _ := cmd.Flags().GetString("name")
	dir, _ := cmd.Flags().GetString("path")
	valid, _ := cmd.Flags().GetStringSlice("valid")
	maxSize, _ := cmd.Flags().GetInt("max-size")

	name = strings.TrimSpace(name)
	if name == "" {
		return cli.UsageError(fmt.Errorf("--name is required"))
	}
	if strings.TrimSpace(dir) == "" {
		return cli.UsageError(fmt.Errorf("--path is required"))
	}
	if maxSize < 0 {
		return cli.UsageError(fmt.Errorf("--max-size must be >= 0, 0 means unlimited"))
	}

	zones, err := config.ReadCacheZones(path)
	if err != nil {
		return configInvalid("cache.add", err)
	}
	valid = cleanValid(valid)
	if len(valid) == 0 {
		return cli.UsageError(fmt.Errorf("--valid is required, e.g. --valid 200:10m or --valid any:30s; a zone without TTLs caches nothing"))
	}
	for _, z := range zones {
		if z.Name == name {
			return &cli.CLIError{Code: cli.CodeConflict, Command: "cache.add",
				Message: fmt.Sprintf("cache zone %q already exists", name)}
		}
	}
	zones = append(zones, config.CacheZone{Name: name, Path: strings.TrimSpace(dir), Valid: valid, MaxSize: maxSize})
	if err := cacheWriteYes(cmd, cli.T("新增缓存区 "+name, "add cache zone "+name)); err != nil {
		return err
	}
	return commit(cmd, "cache.add", "cache add "+name, path, zones, dry)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	name := strings.TrimSpace(strings.TrimSuffix(args[0], ".json"))

	if !cmd.Flags().Changed("path") && !cmd.Flags().Changed("valid") && !cmd.Flags().Changed("max-size") {
		return cli.UsageError(fmt.Errorf("update requires at least one of --path, --valid, --max-size"))
	}

	zones, err := config.ReadCacheZones(path)
	if err != nil {
		return configInvalid("cache.update", err)
	}
	found := false
	for i := range zones {
		if zones[i].Name != name {
			continue
		}
		found = true
		if cmd.Flags().Changed("path") {
			v, _ := cmd.Flags().GetString("path")
			if strings.TrimSpace(v) == "" {
				return cli.UsageError(fmt.Errorf("--path must not be empty"))
			}
			zones[i].Path = strings.TrimSpace(v)
		}
		if cmd.Flags().Changed("valid") {
			v, _ := cmd.Flags().GetStringSlice("valid")
			v = cleanValid(v)
			if len(v) == 0 {
				return cli.UsageError(fmt.Errorf("--valid must not be empty; a zone without TTLs caches nothing"))
			}
			zones[i].Valid = v
		}
		if cmd.Flags().Changed("max-size") {
			v, _ := cmd.Flags().GetInt("max-size")
			if v < 0 {
				return cli.UsageError(fmt.Errorf("--max-size must be >= 0, 0 means unlimited"))
			}
			zones[i].MaxSize = v
		}
		break
	}
	if !found {
		return &cli.CLIError{Code: cli.CodeNotFound, Command: "cache.update",
			Message: fmt.Sprintf("cache zone %q not found", name)}
	}
	if err := cacheWriteYes(cmd, cli.T("修改缓存区 "+name, "update cache zone "+name)); err != nil {
		return err
	}
	return commit(cmd, "cache.update", "cache update "+name, path, zones, dry)
}

func runDelete(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("config")
	dry, _ := cmd.Flags().GetBool("dry-run")
	name := strings.TrimSpace(strings.TrimSuffix(args[0], ".json"))

	zones, err := config.ReadCacheZones(path)
	if err != nil {
		return configInvalid("cache.delete", err)
	}
	out := zones[:0]
	found := false
	var removed config.CacheZone
	for _, z := range zones {
		if z.Name == name {
			found = true
			removed = z
			continue
		}
		out = append(out, z)
	}
	if !found {
		return &cli.CLIError{Code: cli.CodeNotFound, Command: "cache.delete",
			Message: fmt.Sprintf("cache zone %q not found", name)}
	}

	// 仍被站点引用时拒绝：删除后配置会校验失败，与其让事务回滚，
	// 不如在写盘前把引用位置告诉用户。
	refs, err := config.CacheZoneRefs(path, name)
	if err != nil {
		return configInvalid("cache.delete", err)
	}
	if len(refs) > 0 {
		return &cli.CLIError{Code: cli.CodeConflict, Command: "cache.delete",
			Message: fmt.Sprintf("cache zone %q is still referenced by: %s", name, strings.Join(refs, "; "))}
	}
	action := cli.T("删除缓存区 "+name, "delete cache zone "+name)
	if purge, _ := cmd.Flags().GetBool("purge"); purge && !dry && strings.TrimSpace(removed.Path) != "" {
		// --purge 还会删磁盘目录，确认文案里要带上，避免只看到「删配置」。
		action = cli.T("删除缓存区 "+name+" 并清空目录 "+config.DataPath(removed.Path),
			"delete cache zone "+name+" and purge directory "+config.DataPath(removed.Path))
	}
	if err := cacheWriteYes(cmd, action); err != nil {
		return err
	}
	if err := commit(cmd, "cache.delete", "cache delete "+name, path, out, dry); err != nil {
		return err
	}
	if purge, _ := cmd.Flags().GetBool("purge"); purge && !dry {
		purgeDir(cmd, removed.Path)
	}
	return nil
}

// purgeDir 删除缓存区对应的磁盘目录。--purge 是显式选择，但目录为数据根或盘符根时
// 仍然拒绝：那等于把别的数据一起清掉。失败只告警，配置已经删掉了，不回滚。
func purgeDir(cmd *cobra.Command, raw string) {
	dir := strings.TrimSpace(raw)
	if dir == "" {
		return
	}
	dir = config.DataPath(dir)
	cleaned := filepath.Clean(dir)
	if cleaned == "." || cleaned == string(filepath.Separator) || filepath.Dir(cleaned) == cleaned {
		fmt.Fprintf(cmd.ErrOrStderr(),
			cli.T("warning: 拒绝删除可疑的缓存目录 %s\n", "warning: refusing to purge suspicious cache directory %s\n"), cleaned)
		return
	}
	if err := os.RemoveAll(cleaned); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			cli.T("warning: 删除缓存目录 %s 失败：%v\n", "warning: delete cache directory %s failed: %v\n"), cleaned, err)
		return
	}
	if !cli.JSONMode(cmd) {
		fmt.Fprintf(cmd.OutOrStdout(), cli.T("已删除缓存目录 %s\n", "cache directory removed: %s\n"), cleaned)
	}
}

// cacheWriteYes 与 site 写命令一致：dry-run 不需要确认；非 dry-run 时
// 终端里问 y/N，非终端（脚本、桌面端）要求显式 --yes。
func cacheWriteYes(cmd *cobra.Command, action string) error {
	dry, _ := cmd.Flags().GetBool("dry-run")
	if dry {
		return nil
	}
	return cli.ConfirmWrite(cmd, action)
}

// commit 校验并写入缓存区列表；dry-run 只打印改写后的配置。
//
// 非 dry-run 走配置事务：候选树里改主配置 cache 段，校验、临时备份、原子切换
// 与 operation 记录都由事务引擎负责，和 site.* 是同一条链路。
func commit(cmd *cobra.Command, id, summary, path string, zones []config.CacheZone, dry bool) error {
	patched, err := config.PreviewCacheZones(path, zones)
	if err != nil {
		return configInvalid(id, err)
	}
	if dry {
		if cli.JSONMode(cmd) {
			return cli.EmitOK(cmd, id, map[string]any{"dry_run": true, "config": string(patched)}, nil)
		}
		fmt.Fprint(cmd.OutOrStdout(), string(patched))
		return nil
	}

	res, err := cli.RunSectionTxnCmd(cmd, id, summary,
		map[string]string{"config": path, "zones": fmt.Sprintf("%v", zones)},
		func(candMain string) error {
			body, err := config.PreviewCacheZones(candMain, zones)
			if err != nil {
				return configInvalid(id, err)
			}
			return os.WriteFile(candMain, body, 0o644)
		})
	if err != nil {
		return err
	}

	data := map[string]any{
		"config":       path,
		"zones":        zones,
		"operation_id": res.Operation.ID,
		"status":       res.Operation.Status,
		"version":      res.Version,
	}
	if cli.JSONMode(cmd) {
		return cli.EmitOK(cmd, id, data, res.Warnings)
	}
	cli.PrintWarnings(cmd.ErrOrStderr(), res.Warnings)
	fmt.Fprintf(cmd.OutOrStdout(), "缓存区已写入 %s；下次 reload 或启动时生效\n", path)
	return nil
}

// cleanValid 去空白与空项，保持书写顺序。
func cleanValid(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if v := strings.TrimSpace(item); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func configInvalid(id string, err error) error {
	if err == nil {
		return nil
	}
	return &cli.CLIError{Code: cli.CodeConfigInvalid, Command: id, Message: err.Error(), Cause: err}
}
