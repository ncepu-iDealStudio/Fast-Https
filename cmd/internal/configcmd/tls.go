package configcmd

import (
	"fmt"
	"os"
	"strings"

	"fast-https/cmd/internal/cli"
	"fast-https/config"

	"github.com/spf13/cobra"
)

// config tls：配置顶层 tls.default_certificate（未知 SNI 的兜底证书）。
//
// 与 acme 命令同一套路：只替换 tls 段，注释与其它字段原样保留；
// 写入前按主配置完整校验一遍，证书或私钥不存在直接拒绝。
func newConfigTLSCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "tls",
		Short: cli.T("配置 tls.default_certificate（未知 SNI 的兜底证书）", "configure tls.default_certificate (fallback for unknown SNI)"),
		Long: cli.T(
			"写入顶层 tls.default_certificate：certificate 与 key 必须成对，且文件必须存在。\n\n"+
				"未知或空 SNI 的握手会优先使用这张证书；未配置时回落到该端口第一张证书。\n"+
				"两个参数都传空值表示清除该配置。只改写 tls 段，注释与其它字段原样保留。\n"+
				"新配置在下次 reload 或启动时生效；不自动 reload。脚本中非 dry-run 必须加 --yes；在终端里会先询问 y/N。",
			"Write the top-level tls.default_certificate: certificate and key must be\n"+
				"provided together and the files must exist.\n\n"+
				"Handshakes with an unknown or empty SNI prefer this certificate; without it the\n"+
				"first certificate on that port is used. Passing both flags empty clears it.\n"+
				"Only the tls section is rewritten; comments and other fields are preserved.\n"+
				"The change takes effect on the next reload or restart; no reload is triggered.\n"+
				"Unless --dry-run, scripts must pass --yes; interactive terminals are asked y/N first."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigTLSSet,
	}
	cli.AddConfigFlag(c)
	c.Flags().String("cert", "", cli.T("兜底证书路径；与 --key 一起传空表示清除", "fallback certificate path; pass empty together with --key to clear"))
	c.Flags().String("key", "", cli.T("兜底证书私钥路径", "private key path for the fallback certificate"))
	c.Flags().Bool("dry-run", false, cli.T("只打印改写后的配置，不写盘", "print the patched config instead of writing it"))
	c.Flags().Bool("yes", false, cli.T("确认写入（非 dry-run 时，脚本必须显式传入；终端中会询问 y/N）", "confirm the write (required in scripts; interactive terminals are asked y/N)"))

	show := &cobra.Command{
		Use:          "show",
		Short:        cli.T("显示当前的 tls.default_certificate", "show the current tls.default_certificate"),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runConfigTLSShow,
	}
	cli.AddConfigFlag(show)
	c.AddCommand(show)
	return c
}

func runConfigTLSSet(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	cert, _ := cmd.Flags().GetString("cert")
	key, _ := cmd.Flags().GetString("key")

	if !cmd.Flags().Changed("cert") && !cmd.Flags().Changed("key") {
		return cli.UsageError(fmt.Errorf("--cert and --key are required; pass both empty to clear, or use `config tls show`"))
	}
	cert, key = strings.TrimSpace(cert), strings.TrimSpace(key)
	if (cert == "") != (key == "") {
		return cli.UsageError(fmt.Errorf("--cert and --key must be set together"))
	}

	section := &config.TLSConfig{DefaultCertificate: config.Certificate{Certificate: cert, Key: key}}
	patched, err := config.PreviewTLSSection(path, section)
	if err != nil {
		return configInvalid("config.tls", err)
	}
	if dryRun {
		fmt.Fprint(cmd.OutOrStdout(), string(patched))
		return nil
	}
	if err := cli.ConfirmWrite(cmd, cli.T("写入 tls.default_certificate", "write tls.default_certificate")); err != nil {
		return err
	}
	// 与 site.* 一样走配置事务：候选树里改主配置 tls 段，校验、临时备份、
	// 原子切换与 operation 记录都由事务引擎负责。
	if _, err := cli.RunSectionTxnCmd(cmd, "config.tls", "config tls",
		map[string]string{"config": path}, func(candMain string) error {
			body, err := config.PreviewTLSSection(candMain, section)
			if err != nil {
				return configInvalid("config.tls", err)
			}
			return os.WriteFile(candMain, body, 0o644)
		}); err != nil {
		return err
	}
	if cert == "" {
		fmt.Fprintf(cmd.OutOrStdout(), "已清除 tls.default_certificate（%s）；下次 reload 或启动时生效\n", path)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "tls.default_certificate 已写入 %s；下次 reload 或启动时生效\n", path)
	return nil
}

func runConfigTLSShow(cmd *cobra.Command, _ []string) error {
	path, _ := cmd.Flags().GetString("config")
	c, err := config.ReadTLSDefaults(path)
	if err != nil {
		return configInvalid("config.tls", err)
	}
	if !c.HasDefaultCertificate() {
		fmt.Fprintln(cmd.OutOrStdout(), "tls.default_certificate 未配置（未知 SNI 回落到该端口第一张证书）")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "certificate: %s\nkey:         %s\n", c.DefaultCertificate.Certificate, c.DefaultCertificate.Key)
	return nil
}

func configInvalid(id string, err error) *cli.CLIError {
	if err == nil {
		return nil
	}
	return &cli.CLIError{Code: cli.CodeConfigInvalid, Command: id, Message: err.Error(), Cause: err}
}
