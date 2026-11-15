package inspectcmd

import (
	"fast-https/cmd/internal/cli"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"fast-https/management/view"

	"github.com/spf13/cobra"
)

// cert 命令组：只读地查看配置里声明的证书。私钥内容与私钥路径不会出现在任何输出里。
func NewCertCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "cert",
		Short:        cli.T("查看证书状态与检查证书", "list and check certificates"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	root.AddCommand(newCertListCmd(), newCertCheckCmd())
	return root
}

func newCertListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: cli.T("列出配置里声明的证书及其状态", "list the certificates declared in the config and their status"),
		Long: cli.T(
			"按证书文件列出配置里各 ssl 站点声明的证书：域名、状态、到期时间、颁发者、是否由 ACME 管理、被哪些站点使用。\n"+
				"状态：valid、expiring（剩余天数不超过 acme.renew_before_days，默认 30）、expired、not_yet_valid、\n"+
				"missing（文件不存在）、unreadable（读不了或没有证书）、unknown（没有声明证书路径）。\n"+
				"只读取配置里写明的证书路径，不遍历证书目录；不输出私钥内容与私钥路径。",
			"list the certificates declared by the ssl sites in the config, grouped by certificate file:\n"+
				"domains, status, expiry, issuer, whether ACME manages it and which sites use it.\n"+
				"Status: valid, expiring (days left <= acme.renew_before_days, default 30), expired,\n"+
				"not_yet_valid, missing (file not found), unreadable, unknown (no certificate path declared).\n"+
				"Only declared certificate paths are read; private key contents and paths are never printed."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			data, warnings, err := view.CertList(path, time.Now())
			if err != nil {
				return cli.FromViewError("cert.list", err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "cert.list", data, warnings)
			}
			cli.PrintWarnings(cmd.ErrOrStderr(), warnings)
			writeCertListText(cmd.OutOrStdout(), data)
			return nil
		},
	}
	cli.AddConfigFlag(c)
	return c
}

func newCertCheckCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "check",
		Short: cli.T("检查某个域名使用的证书", "check the certificate used by a domain"),
		Long: cli.T(
			"检查 --domain 指定的域名使用的证书：文件是否存在、有效期、证书是否覆盖该域名、证书与私钥是否配对。\n"+
				"检查结果是数据：证书有问题也是一次成功的查询，退出码为 0；域名不在任何 ssl 站点里时退出码为 9。\n"+
				"没能执行的检查（例如当前用户读不了私钥）标为 checked=false，和「执行了但没通过」区分。\n"+
				"目前不做证书链验证，只报告文件里证书的张数。",
			"check the certificate used by the --domain: file existence, validity, whether the certificate\n"+
				"covers the domain, and whether the certificate and private key match.\n"+
				"Results are data: a bad certificate is still a successful query with exit code 0; a domain that\n"+
				"no ssl site declares exits with code 9. A check that could not run (for example the private key\n"+
				"is unreadable) is reported as checked=false, distinct from a check that ran and failed.\n"+
				"Chain validation is not performed; only the number of certificates in the file is reported."),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("config")
			domain, _ := cmd.Flags().GetString("domain")
			if strings.TrimSpace(domain) == "" {
				return cli.UsageError(fmt.Errorf("--domain is required"))
			}
			data, warnings, err := view.CertCheck(path, domain, time.Now())
			if err != nil {
				return cli.FromViewError("cert.check", err)
			}
			if cli.JSONMode(cmd) {
				return cli.EmitOK(cmd, "cert.check", data, warnings)
			}
			cli.PrintWarnings(cmd.ErrOrStderr(), warnings)
			writeCertCheckText(cmd.OutOrStdout(), data)
			return nil
		},
	}
	cli.AddConfigFlag(c)
	c.Flags().String("domain", "", cli.T("要检查的域名（必填）", "domain to check (required)"))
	return c
}

func writeCertListText(w io.Writer, data view.CertListData) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DOMAINS\tSTATUS\tDAYS_LEFT\tACME\tFILE")
	for _, c := range data.Certificates {
		days := "-"
		if c.DaysLeft != nil {
			days = strconv.Itoa(*c.DaysLeft)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\t%s\n", strings.Join(c.Domains, ","), c.Status, days, c.ACMEManaged, c.File)
	}
	_ = tw.Flush()
}

func writeCertCheckText(w io.Writer, data view.CertCheckData) {
	fmt.Fprintf(w, "domain: %s\nstatus: %s\npassed: %t\nfile:   %s\n",
		data.Domain, data.Certificate.Status, data.Passed, data.Certificate.File)
	for _, item := range []struct {
		name string
		c    view.CheckItem
	}{
		{"exists", data.Checks.Exists},
		{"validity", data.Checks.Validity},
		{"hostname", data.Checks.Hostname},
		{"key_pair", data.Checks.KeyPair},
	} {
		state := "not checked"
		switch {
		case item.c.Checked && item.c.OK:
			state = "ok"
		case item.c.Checked:
			state = "FAILED"
		}
		line := fmt.Sprintf("  %-9s %s", item.name+":", state)
		if item.c.Detail != "" {
			line += " (" + item.c.Detail + ")"
		}
		fmt.Fprintln(w, line)
	}
}
