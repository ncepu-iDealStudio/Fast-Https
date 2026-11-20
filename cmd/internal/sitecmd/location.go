package sitecmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"fast-https/cmd/internal/cli"
	"fast-https/config"
	"fast-https/management/siteops"

	"github.com/spf13/cobra"
)

// site location 管理站点的额外 location（例如 / 静态 + /api 反代）。
// 主 location "/" 仍由 site add/update 管理；这里的子命令只增删改其它路径，
// 全部走配置事务，失败自动回滚。
func newLocationCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "location",
		Short: cli.T("管理站点的额外 location（如 /api 反向代理）", "manage extra site locations (e.g. /api reverse proxy)"),
		Long: cli.T(
			"站点文件里的 location 数组可以有多个：/ 继续由 site update 管理，\n"+
				"额外路径用 site location add/update/delete 管理，例如 /api 转发到后端。\n"+
				"匹配按前缀最长优先；proxy_pass 带路径时按 nginx 语义替换命中的前缀。",
			"A site file may contain several locations: / stays under site update, extra paths are managed\n"+
				"with site location add/update/delete, e.g. /api proxied to a backend. Matching is longest-prefix;\n"+
				"when proxy_pass carries a URI, the matched prefix is replaced following nginx semantics."),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newLocationListCmd(), newLocationAddCmd(), newLocationUpdateCmd(), newLocationDeleteCmd(), newLocationSetCmd())
	return root
}

func newLocationListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "list <site>",
		Short:        cli.T("列出站点的全部 location", "list all locations of a site"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runLocationList,
	}
	cli.AddConfigFlag(c)
	return c
}

func newLocationAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add <site>",
		Short: cli.T("新增一个额外 location", "add an extra location"),
		Long: cli.T(
			"新增额外 location。--type proxy 需要 --proxy-pass；--type local 需要 --root（绝对路径）。\n"+
				"路径必须以 / 开头，且不能与已有 location 重复；主 location / 请用 site update 修改。",
			"Add an extra location. --type proxy requires --proxy-pass; --type local requires an absolute --root.\n"+
				"The path must start with / and must not duplicate an existing location; the main / belongs to site update."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runLocationAdd,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("url", "", cli.T("匹配路径，必须以 / 开头，例如 /api", "path prefix, must start with /, e.g. /api"))
	c.Flags().String("type", "proxy", cli.T("location 类型：local 或 proxy", "location type: local or proxy"))
	c.Flags().String("root", "", cli.T("静态根目录（type=local，绝对路径）", "static root (type=local, absolute path)"))
	c.Flags().String("proxy-pass", "", cli.T("上游地址，例如 http://127.0.0.1:9000", "upstream, e.g. http://127.0.0.1:9000"))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不写盘", "validate and show the diff only"))
	return c
}

func newLocationUpdateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "update <site>",
		Short:        cli.T("修改一个额外 location", "update an extra location"),
		Long:         cli.T("按 --url 定位，只改显式给出的 --type / --root / --proxy-pass。", "Located by --url; only the given --type / --root / --proxy-pass are changed."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runLocationUpdate,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("url", "", cli.T("要修改的匹配路径，例如 /api", "path prefix to update, e.g. /api"))
	c.Flags().String("type", "", cli.T("改成 local 或 proxy", "new type: local or proxy"))
	c.Flags().String("root", "", cli.T("新的静态根目录（type=local）", "new static root (type=local)"))
	c.Flags().String("proxy-pass", "", cli.T("新的上游地址（type=proxy）", "new upstream (type=proxy)"))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不写盘", "validate and show the diff only"))
	return c
}

func newLocationDeleteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "delete <site>",
		Short:        cli.T("删除一个额外 location", "delete an extra location"),
		Long:         cli.T("按 --url 删除额外 location；主 location / 不允许删除。必须加 --yes。", "Delete an extra location by --url; the main / cannot be deleted. --yes is required."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runLocationDelete,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("url", "", cli.T("要删除的匹配路径，例如 /api", "path prefix to delete, e.g. /api"))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不写盘", "validate and show the diff only"))
	c.Flags().Bool("yes", false, cli.T("确认删除", "confirm deletion"))
	return c
}

// newLocationSetCmd 供桌面端一次保存：整体替换非主 location。
// 每个 --location 是一个 JSON 对象：{"url":"/api","type":"proxy","proxy_pass":"http://..."}。
func newLocationSetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:          "set <site>",
		Short:        cli.T("整体替换站点的额外 location", "replace all extra locations of a site"),
		Long:         cli.T("主 location / 保持不动；--location 可重复，不传表示清空额外 location。", "The main / stays untouched; --location is repeatable and an empty list clears extra locations."),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runLocationSet,
	}
	cli.AddConfigFlag(c)
	c.Flags().StringArray("location", nil, cli.T(`额外 location 的 JSON，例如 {"url":"/api","type":"proxy","proxy_pass":"http://127.0.0.1:9000"}`, `extra location JSON, e.g. {"url":"/api","type":"proxy","proxy_pass":"http://127.0.0.1:9000"}`))
	c.Flags().Bool("dry-run", false, cli.T("只校验与展示差异，不写盘", "validate and show the diff only"))
	return c
}

func runLocationSet(cmd *cobra.Command, args []string) error {
	live, _ := cmd.Flags().GetString("config")
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	rawLocs, _ := cmd.Flags().GetStringArray("location")
	dry, _ := cmd.Flags().GetBool("dry-run")
	extras := make([]config.SiteLocation, 0, len(rawLocs))
	for _, raw := range rawLocs {
		var loc config.SiteLocation
		if err := json.Unmarshal([]byte(raw), &loc); err != nil {
			return cli.UsageError(fmt.Errorf("invalid --location JSON %q: %w", raw, err))
		}
		extras = append(extras, loc)
	}
	return cli.RunTxn(cmd, "site.location.set", "site location set "+name,
		map[string]string{"site": name, "locations": strings.Join(rawLocs, "\n")}, dry,
		func(_ string, dirs []string) error {
			_, err := siteops.SetExtraLocations(dirs, live, name, extras, cli.BannedRoots(live))
			return err
		})
}

func runLocationList(cmd *cobra.Command, args []string) error {
	live, _ := cmd.Flags().GetString("config")
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	dirs, err := config.SiteDirsFromFile(live)
	if err != nil {
		return cli.FromViewError("site.location.list", err)
	}
	locs, err := siteops.ListLocations(dirs, name)
	if err != nil {
		return cli.FromSiteopsError("site.location.list", err)
	}
	if cli.JSONMode(cmd) {
		return cli.EmitOK(cmd, "site.location.list", map[string]any{"site": name, "locations": locs}, nil)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-16s %-8s %s\n", "URL", "TYPE", "TARGET")
	for _, l := range locs {
		target := l.Root
		if l.Type == "proxy" {
			target = l.ProxyPass
		}
		fmt.Fprintf(out, "%-16s %-8s %s\n", strings.TrimSpace(l.URL), l.Type, target)
	}
	return nil
}

func runLocationAdd(cmd *cobra.Command, args []string) error {
	live, _ := cmd.Flags().GetString("config")
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	url, _ := cmd.Flags().GetString("url")
	typ, _ := cmd.Flags().GetString("type")
	rootFlag, _ := cmd.Flags().GetString("root")
	pass, _ := cmd.Flags().GetString("proxy-pass")
	dry, _ := cmd.Flags().GetBool("dry-run")
	in := siteops.LocationInput{
		Site: name, URL: url, Type: typ, Root: rootFlag, ProxyPass: pass,
		Banned: cli.BannedRoots(live),
	}
	return cli.RunTxn(cmd, "site.location.add", "site location add "+name+" "+strings.TrimSpace(url),
		map[string]string{"site": name, "url": url, "type": typ}, dry,
		func(_ string, dirs []string) error {
			_, err := siteops.AddLocation(dirs, live, in)
			return err
		})
}

func runLocationUpdate(cmd *cobra.Command, args []string) error {
	live, _ := cmd.Flags().GetString("config")
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	url, _ := cmd.Flags().GetString("url")
	dry, _ := cmd.Flags().GetBool("dry-run")
	in := siteops.LocationUpdateInput{Site: name, URL: url, Banned: cli.BannedRoots(live)}
	if cmd.Flags().Changed("type") {
		v, _ := cmd.Flags().GetString("type")
		in.Type = &v
	}
	if cmd.Flags().Changed("root") {
		v, _ := cmd.Flags().GetString("root")
		in.Root = &v
	}
	if cmd.Flags().Changed("proxy-pass") {
		v, _ := cmd.Flags().GetString("proxy-pass")
		in.ProxyPass = &v
	}
	return cli.RunTxn(cmd, "site.location.update", "site location update "+name+" "+strings.TrimSpace(url),
		map[string]string{"site": name, "url": url}, dry,
		func(_ string, dirs []string) error {
			_, err := siteops.UpdateLocation(dirs, live, in)
			return err
		})
}

func runLocationDelete(cmd *cobra.Command, args []string) error {
	if !cli.Confirmed(cmd) {
		return cli.UsageError(fmt.Errorf("location delete requires --yes"))
	}
	live, _ := cmd.Flags().GetString("config")
	name := strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	url, _ := cmd.Flags().GetString("url")
	dry, _ := cmd.Flags().GetBool("dry-run")
	return cli.RunTxn(cmd, "site.location.delete", "site location delete "+name+" "+strings.TrimSpace(url),
		map[string]string{"site": name, "url": url}, dry,
		func(_ string, dirs []string) error {
			_, err := siteops.DeleteLocation(dirs, live, name, url)
			return err
		})
}
