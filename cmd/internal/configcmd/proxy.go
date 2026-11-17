package configcmd

import (
	"fmt"
	"strings"

	"fast-https/cmd/internal/cli"
	"fast-https/config"

	"github.com/spf13/cobra"
)

// config proxy：配置顶层 proxy 段（可信代理、XFF 链长上限、Host 透传）。
//
// 与 config tls 同一套路：只替换 proxy 段，注释与其它字段原样保留；
// 写入前按主配置完整校验一遍。只传部分参数时在现有值上打补丁。
func newConfigProxyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "proxy",
		Short: cli.T("配置可信代理、X-Forwarded-For 链长与 Host 透传", "configure trusted proxies, X-Forwarded-For hops and Host preservation"),
		Long: cli.T(
			"写入顶层 proxy 段。默认只追加真实客户端地址，不信任客户端自带的 X-Forwarded-For；\n"+
				"只有来自 trusted_proxies 的请求才保留入站链路。\n\n"+
				"--trusted-proxies 接受 IP 或 CIDR，逗号分隔；传空值表示清空。\n"+
				"--max-hops 为 0 表示不限制，达到上限后不再追加并记一条告警日志。\n"+
				"--preserve-host 默认 true（原样透传）；false 时改写成上游地址。\n"+
				"--upstream-timeout 是代理到上游的整体超时（秒），0 表示默认 20 秒。\n"+
				"--tunnel-idle-timeout 是 websocket / tcp 隧道的空闲断开时间（秒），0 表示不断开。\n\n"+
				"五项都回到默认值时删除整个 proxy 段。只改写 proxy 段，注释与其它字段原样保留。\n"+
				"新配置在下次 reload 或启动时生效；不自动 reload。",
			"Write the top-level proxy section. By default only the real client address is appended;\n"+
				"an inbound X-Forwarded-For is kept only when the peer matches trusted_proxies.\n\n"+
				"--trusted-proxies accepts IPs or CIDRs, comma separated; an empty value clears it.\n"+
				"--max-hops 0 means unlimited; once the limit is reached the address is not appended\n"+
				"and a warning is logged.\n"+
				"--preserve-host defaults to true (pass through); false rewrites Host to the upstream address.\n"+
				"--upstream-timeout is the overall timeout to the upstream in seconds; 0 uses the default 20s.\n"+
				"--tunnel-idle-timeout closes an idle websocket / tcp tunnel after N seconds; 0 disables it.\n\n"+
				"When all five are back to defaults the whole proxy section is removed. Only the proxy\n"+
				"section is rewritten; comments and other fields are preserved.\n"+
				"The change takes effect on the next reload or restart; no reload is triggered."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigProxySet,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("trusted-proxies", "", cli.T("可信代理 IP/CIDR，逗号分隔；传空表示清空", "trusted proxy IPs/CIDRs, comma separated; empty clears"))
	c.Flags().Int("max-hops", 0, cli.T("X-Forwarded-For 链长上限，0 表示不限制", "X-Forwarded-For hop limit, 0 means unlimited"))
	c.Flags().Bool("preserve-host", true, cli.T("透传客户端 Host；false 时改写为上游地址", "pass the client Host through; false rewrites it to the upstream address"))
	c.Flags().Int("upstream-timeout", 0, cli.T("代理到上游的超时（秒），0 表示默认 20 秒", "upstream timeout in seconds; 0 uses the default 20s"))
	c.Flags().Int("tunnel-idle-timeout", 0, cli.T("websocket / tcp 隧道空闲断开时间（秒），0 表示不断开", "idle timeout for websocket / tcp tunnels in seconds; 0 disables it"))
	c.Flags().Bool("dry-run", false, cli.T("只打印改写后的配置，不写盘", "print the patched config instead of writing it"))

	show := &cobra.Command{
		Use:          "show",
		Short:        cli.T("显示当前的 proxy 配置", "show the current proxy configuration"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigProxyShow,
	}
	cli.AddConfigFlag(show)
	c.AddCommand(show)
	return c
}

func runConfigProxySet(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if !cmd.Flags().Changed("trusted-proxies") && !cmd.Flags().Changed("max-hops") &&
		!cmd.Flags().Changed("preserve-host") && !cmd.Flags().Changed("upstream-timeout") &&
		!cmd.Flags().Changed("tunnel-idle-timeout") {
		return cli.UsageError(fmt.Errorf("at least one proxy flag is required; use `config proxy show` to inspect"))
	}

	section, err := config.ReadProxySection(path)
	if err != nil {
		return configInvalid("config.proxy", err)
	}

	if cmd.Flags().Changed("trusted-proxies") {
		raw, _ := cmd.Flags().GetString("trusted-proxies")
		section.TrustedProxies = splitList(raw)
	}
	if cmd.Flags().Changed("max-hops") {
		maxHops, _ := cmd.Flags().GetInt("max-hops")
		if maxHops < 0 {
			return cli.UsageError(fmt.Errorf("--max-hops must be >= 0, 0 means unlimited"))
		}
		section.ForwardedFor.MaxHops = maxHops
	}
	if cmd.Flags().Changed("preserve-host") {
		preserve, _ := cmd.Flags().GetBool("preserve-host")
		section.PreserveHost = &preserve
	}
	if cmd.Flags().Changed("upstream-timeout") {
		timeout, _ := cmd.Flags().GetInt("upstream-timeout")
		if timeout < 0 {
			return cli.UsageError(fmt.Errorf("--upstream-timeout must be >= 0, 0 means the default 20s"))
		}
		section.UpstreamTimeout = timeout
	}
	if cmd.Flags().Changed("tunnel-idle-timeout") {
		timeout, _ := cmd.Flags().GetInt("tunnel-idle-timeout")
		if timeout < 0 {
			return cli.UsageError(fmt.Errorf("--tunnel-idle-timeout must be >= 0, 0 disables idle closing"))
		}
		section.TunnelIdleTimeout = timeout
	}

	patched, err := config.PreviewProxySection(path, &section)
	if err != nil {
		return configInvalid("config.proxy", err)
	}
	if dryRun {
		fmt.Fprint(cmd.OutOrStdout(), string(patched))
		return nil
	}
	// 完整校验在写盘之前：非法 CIDR、负数链长都在这里拦下。
	if err := validatePatchedConfig(path, patched); err != nil {
		return configInvalid("config.proxy", err)
	}
	if err := config.WriteProxySection(path, &section); err != nil {
		return configInvalid("config.proxy", err)
	}
	if config.IsDefaultProxy(section) {
		fmt.Fprintf(cmd.OutOrStdout(), "已清除 proxy 配置（%s）；下次 reload 或启动时生效\n", path)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "proxy 配置已写入 %s；下次 reload 或启动时生效\n", path)
	return nil
}

func runConfigProxyShow(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	section, err := config.ReadProxySection(path)
	if err != nil {
		return configInvalid("config.proxy", err)
	}
	if config.IsDefaultProxy(section) {
		fmt.Fprintln(cmd.OutOrStdout(), "proxy 未配置：只追加真实客户端地址，链长不限，Host 原样透传，上游超时 20 秒")
		return nil
	}
	trusted := "（空）"
	if len(section.TrustedProxies) > 0 {
		trusted = strings.Join(section.TrustedProxies, ", ")
	}
	upstream := section.UpstreamTimeout
	if upstream == 0 {
		upstream = 20
	}
	fmt.Fprintf(cmd.OutOrStdout(), "trusted_proxies:  %s\nmax_hops:         %d\npreserve_host:    %t\nupstream_timeout: %d\ntunnel_idle_timeout: %d\n",
		trusted, section.ForwardedFor.MaxHops, section.PreserveHostEnabled(), upstream, section.TunnelIdleTimeout)
	return nil
}

// splitList 解析逗号分隔的列表，忽略空白项。
func splitList(raw string) []string {
	var out []string
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
