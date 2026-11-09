package acmecmd

import (
	"errors"
	"fast-https/cmd/internal/cli"
	"fast-https/config"
	"fast-https/modules/certmanager"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// ACME 配置命令：acme（交互向导）、acme setup（flags 可非交互）、acme show。
// 写回配置文件时只替换顶层 acme 段，注释与其它字段原样保留
// （见 config.WriteACMESection）。

type acmeCAChoice string

const (
	acmeCALetsEncrypt acmeCAChoice = "letsencrypt"
	acmeCAStaging     acmeCAChoice = "staging"
	acmeCAZeroSSL     acmeCAChoice = "zerossl"
	acmeCACustom      acmeCAChoice = "custom"
)

// acmeAnswers 是向导与 flags 的共同输入，buildConfig 是唯一的出口。
type acmeAnswers struct {
	Enabled         bool
	Email           string
	CA              acmeCAChoice
	Directory       string
	EABKid          string
	EABHmacKey      string
	Challenge       string
	RenewBeforeDays int
	Storage         string
	Fallback        bool
}

// acmeOptions 是命令层可注入的运行时依赖，测试用它替换 stdin/stdout。
type acmeOptions struct {
	Path           string
	NonInteractive bool
	DryRun         bool
	// Accessible 强制 huh 走逐行问答模式。测试里用它避开 tty 检测，
	// 生产环境由 stdout 是否终端自动决定。
	Accessible bool
	In         io.Reader
	Out        io.Writer
}

func NewAcmeCmd() *cobra.Command {
	wizard := &cobra.Command{
		Use:   "acme",
		Short: cli.T("交互式配置 ACME 证书签发", "configure ACME certificate issuance interactively"),
		Long: cli.T(
			"配置 ACME（Let's Encrypt / ZeroSSL）证书签发。\n\n"+
				"不带子命令运行进入交互向导：只改写配置文件顶层的 \"acme\" 段，注释原样保留。\n"+
				"新配置在下次 reload 或启动时生效。",
			"Configure ACME (Let's Encrypt / ZeroSSL) certificate issuance.\n\n"+
				"Running it without a subcommand starts the interactive wizard. It only rewrites\n"+
				"the top-level \"acme\" section of the config file and keeps every comment intact.\n"+
				"The new settings take effect on the next reload or restart."),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return runAcmeSetup(cmd) },
	}

	setup := &cobra.Command{
		Use:          "setup",
		Short:        cli.T("写入 acme 段（不加 --non-interactive 时进入交互）", "write the acme section (interactive unless --non-interactive)"),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return runAcmeSetup(cmd) },
	}

	show := &cobra.Command{
		Use:          "show",
		Short:        cli.T("打印当前 acme 设置与将要申请的域名", "print the effective acme settings and the domains that will be requested"),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAcmeShow(cmd.OutOrStdout())
		},
	}

	// 交互向导只留 --config：CA、邮箱、挑战方式这些都在问答里逐项给出默认值，
	// 把十几个 flag 摆在 `acme --help` 里只会让「跑一次向导」这件事看起来很难。
	// 需要脚本化时再用 `acme setup`，那里的 flag 一个不少。
	wizard.Flags().String("config", config.CONFIG_FILE_PATH, cli.T("要改写的配置文件", "config file to patch"))
	registerAcmeSetupFlags(setup)

	wizard.AddCommand(setup, show)
	return wizard
}

func registerAcmeSetupFlags(c *cobra.Command) {
	f := c.Flags()
	f.String("email", "", cli.T("账户邮箱，启用 ACME 时必填", "account email, required when ACME is enabled"))
	f.String("ca", string(acmeCALetsEncrypt), cli.T("letsencrypt | staging | zerossl | custom", "letsencrypt | staging | zerossl | custom"))
	f.String("directory", "", cli.T("ACME directory URL，配合 --ca=custom 使用", "ACME directory URL, used with --ca=custom"))
	f.String("eab-kid", "", cli.T("外部账户绑定 key id（ZeroSSL）", "external account binding key id (ZeroSSL)"))
	f.String("eab-hmac-key", "", cli.T("外部账户绑定 HMAC key（ZeroSSL）", "external account binding HMAC key (ZeroSSL)"))
	f.String("challenge", config.ACMEChallengeAuto, cli.T("auto | http-01 | tls-alpn-01", "auto | http-01 | tls-alpn-01"))
	f.Int("renew-before-days", config.DefaultACMERenewBeforeDays, cli.T("到期前多少天开始续期", "renew this many days before expiry"))
	f.String("storage", config.ACMECertStorageDir, cli.T("ACME 账户与订单缓存的目录", "directory for the ACME account and order cache"))
	f.Bool("fallback-self-signed", true, cli.T("签发失败时继续用自签证书", "keep serving the self-signed certificate when issuance fails"))
	f.Bool("non-interactive", false, cli.T("全部取值来自参数，不做任何提问", "take every value from flags, never prompt"))
	f.Bool("plain", false, cli.T("使用逐行问答代替全屏表单", "plain line-by-line prompts instead of the full-screen form"))
	f.Bool("dry-run", false, cli.T("打印将要写入的配置而不是写盘", "print the patched config instead of writing it"))
	f.String("config", config.CONFIG_FILE_PATH, cli.T("要改写的配置文件", "config file to patch"))
}

// runAcmeSetup 从 flags 组装答案，交互模式下再用向导补齐。
func runAcmeSetup(cmd *cobra.Command) error {
	// 先把当前配置读进内存：向导的默认值与摘要里的域名清单都来自它。
	// 读不到时继续走（写盘只需要文件本身可解析），所以忽略错误。
	if err := config.Init(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "读取当前配置失败，向导默认值为空：%v\n", err)
	}

	flags := cmd.Flags()
	path, _ := flags.GetString("config")
	dryRun, _ := flags.GetBool("dry-run")
	nonInteractive, _ := flags.GetBool("non-interactive")

	// 默认值一律来自当前配置：再跑一次 acme 时，上次选的 CA、挑战方式、
	// 续期天数、缓存目录与回落开关都应该原样带出来，而不是退回出厂值。
	answers := acmeAnswersFromConfig(config.Current())
	// 只有显式传了的 flag 才覆盖配置值，其余保持继承。
	overrideACMEAnswers(&answers, cmd)

	plain, _ := flags.GetBool("plain")
	opts := acmeOptions{
		Path:   path,
		DryRun: dryRun,
		// --plain 强制走逐行问答：一题一行，适合脚本录制或不喜欢全屏 TUI 的场景。
		Accessible: plain,
		// 标准输入不是终端（systemd、容器、管道）时无法交互，
		// 直接走 flags 路径，避免命令卡在等输入上。
		NonInteractive: nonInteractive || !cli.InteractiveAvailable(),
		In:             cmd.InOrStdin(),
		Out:            cmd.OutOrStdout(),
	}
	return setupACME(answers, opts)
}

// setupACME 跑完向导、校验、写盘，并把下一步提示打出来。
func setupACME(answers acmeAnswers, opts acmeOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Path == "" {
		opts.Path = config.CONFIG_FILE_PATH
	}

	if !opts.NonInteractive {
		if err := promptACME(&answers, opts); err != nil {
			return err
		}
	} else if answers.Enabled && strings.TrimSpace(answers.Email) == "" {
		// 非 TTY（systemd、管道、CI）时无法提问，直接说清楚缺什么，
		// 别让用户对着「邮箱不能为空」猜为什么没进向导。
		return errors.New("标准输入不是终端或已指定 --non-interactive：请用 --email 提供账户邮箱，" +
			"例如 fast-https acme setup --email ops@example.com --non-interactive")
	}

	cfg, err := answers.buildConfig()
	if err != nil {
		return err
	}

	if opts.DryRun {
		patched, err := config.PreviewACMESection(opts.Path, cfg)
		if err != nil {
			return err
		}
		fmt.Fprint(opts.Out, string(patched))
		return nil
	}

	if err := config.WriteACMESection(opts.Path, cfg); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "acme 配置已写入 %s\n", opts.Path)
	printACMENextSteps(opts.Out, cfg, answers)
	return nil
}

// promptACME 分三段提问：条件字段（EAB/自定义目录）放在第二段，
// 这样第二段的字段集合由第一段的选择决定，不会被无关问题打断。
func promptACME(answers *acmeAnswers, opts acmeOptions) error {
	caOptions := []huh.Option[acmeCAChoice]{
		huh.NewOption("Let's Encrypt（默认，免费，90 天）", acmeCALetsEncrypt),
		huh.NewOption("Let's Encrypt Staging（联调用，证书不受信任）", acmeCAStaging),
		huh.NewOption("ZeroSSL（需要 EAB 凭据）", acmeCAZeroSSL),
		huh.NewOption("自定义 ACME 目录", acmeCACustom),
	}
	challengeOptions := []huh.Option[string]{
		huh.NewOption("auto（两者都开，按可用性自动选择）", config.ACMEChallengeAuto),
		huh.NewOption("http-01（需要外部能访问 80）", config.ACMEChallengeHTTP01),
		huh.NewOption("tls-alpn-01（需要外部能直连 443）", config.ACMEChallengeTLSALPN01),
	}
	renewOptions := []huh.Option[int]{
		huh.NewOption("30 天（默认）", 30),
		huh.NewOption("15 天", 15),
		huh.NewOption("45 天", 45),
		huh.NewOption("60 天", 60),
	}

	// 每个字段单独成组：huh 一次只渲染一组，于是变成"一题一屏"，
	// 答完当前题回车进入下一题。多个字段塞进同一组会变成上下排列的一张长表单。
	basic := huh.NewForm(
		huh.NewGroup(huh.NewConfirm().
			Title("启用 ACME 自动申请证书？").
			Description("关闭后仍使用自签证书，配置段的其它字段保留。").
			Affirmative("启用").
			Negative("关闭").
			Value(&answers.Enabled)),
		huh.NewGroup(huh.NewInput().
			Title("账户邮箱").
			Description("CA 用它登记账户并在证书过期前通知。").
			Placeholder("ops@example.com").
			Value(&answers.Email).
			Validate(validateEmail)),
		huh.NewGroup(huh.NewSelect[acmeCAChoice]().
			Title("用哪个 CA？").
			Options(caOptions...).
			Value(&answers.CA)),
	)

	extra := extraACMEForm(answers)

	advanced := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().
			Title("挑战方式").
			Description("两种挑战都由本进程应答，不额外占用端口。").
			Options(challengeOptions...).
			Value(&answers.Challenge)),
		huh.NewGroup(huh.NewSelect[int]().
			Title("到期前多少天续期？").
			Options(renewOptions...).
			Value(&answers.RenewBeforeDays)),
		huh.NewGroup(huh.NewInput().
			Title("账户与订单缓存目录").
			Description(fmt.Sprintf("相对路径按 %s 解析，与二进制所在目录无关；包布局下实际是 %s。",
				config.FAST_HTTPS_ETC_DIR, filepath.Join(config.FAST_HTTPS_ETC_DIR, "cert", "acme"))).
			Placeholder(config.ACMECertStorageDir).
			Value(&answers.Storage).
			Validate(cli.ValidateNotBlank)),
		huh.NewGroup(huh.NewConfirm().
			Title("申请失败时继续用自签证书？").
			Description("关掉后证书申请失败会导致启动告警并且没有 HTTPS 证书可用。").
			Affirmative("是").
			Negative("否").
			Value(&answers.Fallback)),
	)

	// 顺序很关键：摘要必须等前三段答完再生成。
	// huh 的 Note Description 是创建表单时就求值的静态字符串，
	// 提前构造 confirm 会把打开向导前的旧值写进"即将写入"，
	// 用户明明填了邮箱，摘要里却还是空的。
	for _, form := range []*huh.Form{basic, extra, advanced} {
		if form == nil {
			continue
		}
		if err := runACMEForm(form, opts); err != nil {
			return err
		}
	}

	confirmValue := true
	confirm := huh.NewForm(
		// Note 也单独一组：先让人看完整摘要，翻页后再确认。
		huh.NewGroup(huh.NewNote().
			Title("即将写入").
			Description(acmeSummary(*answers))),
		huh.NewGroup(huh.NewConfirm().
			Title("确认写入配置文件？").
			Affirmative("写入").
			Negative("取消").
			Value(&confirmValue)),
	)
	if err := runACMEForm(confirm, opts); err != nil {
		return err
	}
	if !confirmValue {
		return errors.New("已取消，配置未改动")
	}
	answers.Email = strings.TrimSpace(answers.Email)
	answers.Storage = strings.TrimSpace(answers.Storage)
	return nil
}

func runACMEForm(form *huh.Form, opts acmeOptions) error {
	cli.ApplyStdio(form, opts.In, opts.Out, opts.Accessible)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("已取消，配置未改动")
		}
		return err
	}
	return nil
}

// extraACMEForm 只为 ZeroSSL 与自定义目录准备字段，其余 CA 返回 nil。
func extraACMEForm(answers *acmeAnswers) *huh.Form {
	switch answers.CA {
	case acmeCAZeroSSL:
		return huh.NewForm(
			huh.NewGroup(huh.NewInput().
				Title("EAB Key ID").
				Description("ZeroSSL 后台的 External Account Binding 凭据。").
				Value(&answers.EABKid).
				Validate(cli.ValidateNotBlank)),
			huh.NewGroup(huh.NewInput().
				Title("EAB HMAC Key").
				Description("与 Key ID 配对，粘贴时不会回显。").
				EchoMode(huh.EchoModePassword).
				Value(&answers.EABHmacKey).
				Validate(cli.ValidateNotBlank)),
		)
	case acmeCACustom:
		return huh.NewForm(huh.NewGroup(huh.NewInput().
			Title("ACME 目录地址").
			Placeholder("https://acme.example.com/directory").
			Value(&answers.Directory).
			Validate(validateDirectory)))
	}
	return nil
}
func validateEmail(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("邮箱不能为空：CA 不接受匿名注册")
	}
	at := strings.LastIndex(v, "@")
	if at <= 0 || at == len(v)-1 || !strings.Contains(v[at+1:], ".") {
		return errors.New("邮箱格式不对，例如 ops@example.com")
	}
	return nil
}
func validateDirectory(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("目录地址不能为空")
	}
	if !strings.HasPrefix(v, "https://") {
		return errors.New("目录地址应以 https:// 开头")
	}
	return nil
}

// buildConfig 把答案翻成 acme 段；所有校验集中在写盘之前。
func (a acmeAnswers) buildConfig() (*config.AcmeConfig, error) {
	switch a.CA {
	case acmeCALetsEncrypt, acmeCAStaging, acmeCAZeroSSL, acmeCACustom:
	default:
		return nil, fmt.Errorf("未知的 CA：%q（可选 letsencrypt、staging、zerossl、custom）", a.CA)
	}
	switch a.Challenge {
	case config.ACMEChallengeAuto, config.ACMEChallengeHTTP01, config.ACMEChallengeTLSALPN01:
	default:
		return nil, fmt.Errorf("未知的挑战方式：%q（可选 auto、http-01、tls-alpn-01）", a.Challenge)
	}
	if a.RenewBeforeDays <= 0 {
		return nil, fmt.Errorf("renew_before_days 必须大于 0，当前 %d", a.RenewBeforeDays)
	}
	if a.Enabled {
		if err := validateEmail(a.Email); err != nil {
			return nil, err
		}
	}

	fallback := a.Fallback
	cfg := &config.AcmeConfig{
		Enabled:            a.Enabled,
		Email:              strings.TrimSpace(a.Email),
		Staging:            a.CA == acmeCAStaging,
		Challenge:          []string{a.Challenge},
		RenewBeforeDays:    a.RenewBeforeDays,
		FallbackSelfSigned: &fallback,
		Storage:            strings.TrimSpace(a.Storage),
	}
	if cfg.Storage == "" {
		cfg.Storage = config.ACMECertStorageDir
	}

	switch a.CA {
	case acmeCALetsEncrypt, acmeCAStaging:
		cfg.CAs = []config.ACMECA{{Name: "letsencrypt", Directory: config.ACMELetsEncryptDirectory}}
	case acmeCAZeroSSL:
		if strings.TrimSpace(a.EABKid) == "" || strings.TrimSpace(a.EABHmacKey) == "" {
			return nil, errors.New("ZeroSSL 需要 eab_kid 与 eab_hmac_key，两个都要填")
		}
		cfg.CAs = []config.ACMECA{{
			Name:       "zerossl",
			Directory:  config.ACMEZeroSSLDirectory,
			EABKid:     strings.TrimSpace(a.EABKid),
			EABHmacKey: strings.TrimSpace(a.EABHmacKey),
		}}
	case acmeCACustom:
		if err := validateDirectory(a.Directory); err != nil {
			return nil, err
		}
		cfg.CAs = []config.ACMECA{{Name: "custom", Directory: strings.TrimSpace(a.Directory)}}
	}
	return cfg, nil
}

// acmeSummary 是确认页的摘要：写什么、哪些域名会被申请、哪些会被跳过。
func acmeSummary(a acmeAnswers) string {
	var b strings.Builder
	state := "关闭"
	if a.Enabled {
		state = "启用"
	}
	email := cmpOr(a.Email, "(未填)")
	if a.Enabled && strings.TrimSpace(a.Email) == "" {
		email = "(未填，启用 ACME 时必须填写)"
	}
	ca := string(a.CA)
	if a.CA == acmeCACustom {
		ca = fmt.Sprintf("custom（%s）", a.Directory)
	}
	fmt.Fprintf(&b, "启用：%s\n邮箱：%s\nCA：%s\n挑战：%s\n续期提前：%d 天\n缓存目录：%s\n失败回落自签：%v\n",
		state, email, ca, a.Challenge, a.RenewBeforeDays,
		cmpOr(a.Storage, config.ACMECertStorageDir), a.Fallback)

	plan := certmanager.Plan(config.Current())
	if len(plan.Managed) > 0 {
		fmt.Fprintf(&b, "\n会申请证书的域名：%s\n", strings.Join(plan.Managed, ", "))
	} else {
		b.WriteString("\n当前没有可用于 ACME 的域名（需要 listen 含 ssl 的公网域名）。\n")
	}
	for _, skip := range plan.Skipped {
		fmt.Fprintf(&b, "跳过 %s：%s\n", skip.Name, skip.Reason)
	}
	return b.String()
}

func printACMENextSteps(out io.Writer, cfg *config.AcmeConfig, a acmeAnswers) {
	if !cfg.Enabled {
		fmt.Fprintln(out, "ACME 未启用，服务继续使用自签证书。")
		return
	}
	if a.CA == acmeCAStaging {
		fmt.Fprintln(out, "注意：当前使用 Let's Encrypt 测试环境，签发的证书不受浏览器信任。")
	}
	// 没有可申请的域名时，重启也不会有任何证书请求。必须把原因说清，
	// 否则用户只看到"acme 配置已写入"，会以为已经配好了。
	if current := config.Current(); current != nil && len(current.Servers) > 0 {
		plan := certmanager.Plan(current)
		if len(plan.Managed) == 0 {
			fmt.Fprintln(out, "注意：当前没有可申请的域名，重启后 ACME 不会发出任何证书请求。")
			fmt.Fprintln(out, "  只有 listen 里带 ssl 的站点才会被申请证书。给站点加 443，例如编辑 conf.d/<站点>.json：")
			fmt.Fprintln(out, `    "listen": [80, "443 ssl h2"],`)
			fmt.Fprintln(out, `    "ssl_certificate": "./cert/<域名>.pem",`)
			fmt.Fprintln(out, `    "ssl_certificate_key": "./cert/<域名>-key.pem"`)
			fmt.Fprintln(out, `  （或重建站点：fast-https site add --domain <域名> --listen 80 --listen "443 ssl h2" --tls --yes）`)
			for _, skip := range plan.Skipped {
				fmt.Fprintf(out, "  跳过 %s：%s\n", skip.Name, skip.Reason)
			}
		}
	}
	fmt.Fprintln(out, "acme 段在下次 reload 或启动时生效，不需要重启进程：")
	fmt.Fprintln(out, "  fast-https reload    # 或 systemctl reload fast-https")
	fmt.Fprintln(out, "reload 后日志里应出现 acme: requesting certificate，签发成功后证书会热替换。")
}

func runAcmeShow(out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	if err := config.Init(); err != nil {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	writeACMEShow(out, config.Current().Acme, certmanager.Plan(config.Current()))
	return nil
}

// writeACMEShow 只依赖传入的快照与域名划分，便于测试脱敏与跳过原因。
func writeACMEShow(out io.Writer, cfg *config.AcmeConfig, plan certmanager.DomainPlan) {
	if cfg == nil {
		fmt.Fprintln(out, "未配置 acme 段，当前使用自签证书。运行 fast-https acme 生成配置。")
		return
	}

	state := "关闭"
	if cfg.Enabled {
		state = "启用"
	}
	fmt.Fprintf(out, "状态：%s\n", state)
	fmt.Fprintf(out, "邮箱：%s\n", cmpOr(cfg.Email, "(空)"))
	fmt.Fprintf(out, "测试环境（staging）：%v\n", cfg.Staging)
	fmt.Fprintf(out, "挑战：%s\n", strings.Join(cfg.Challenge, ", "))
	fmt.Fprintf(out, "续期提前：%d 天\n", cfg.RenewBeforeDays)
	fmt.Fprintf(out, "缓存目录：%s\n", cfg.Storage)
	fmt.Fprintf(out, "失败回落自签：%v\n", cfg.FallbackSelfSignedEnabled())
	fmt.Fprintln(out, "CA 降级链：")
	for _, ca := range cfg.CAs {
		if ca.EABHmacKey != "" {
			fmt.Fprintf(out, "  - %s %s (eab_kid=%s, eab_hmac_key=%s)\n",
				ca.Name, ca.Directory, cmpOr(ca.EABKid, "-"), maskSecret(ca.EABHmacKey))
			continue
		}
		fmt.Fprintf(out, "  - %s %s\n", ca.Name, ca.Directory)
	}

	if len(plan.Managed) == 0 {
		fmt.Fprintln(out, "将申请的域名：(无)")
	} else {
		fmt.Fprintf(out, "将申请的域名：%s\n", strings.Join(plan.Managed, ", "))
	}
	for _, skip := range plan.Skipped {
		fmt.Fprintf(out, "跳过 %s：%s\n", skip.Name, skip.Reason)
	}
}

// maskSecret 只露出尾部 4 位，避免截图或粘贴日志时泄露 EAB 私钥。
func maskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

// acmeAnswersFromConfig 用当前配置里的 acme 段填充向导默认值。
// 没有 acme 段（首次配置）时才退回出厂默认：启用、Let's Encrypt、auto、30 天。
func acmeAnswersFromConfig(current *config.Fast_Https) acmeAnswers {
	a := acmeAnswers{
		Enabled:         true,
		CA:              acmeCALetsEncrypt,
		Challenge:       config.ACMEChallengeAuto,
		RenewBeforeDays: config.DefaultACMERenewBeforeDays,
		Storage:         config.ACMECertStorageDir,
		Fallback:        true,
	}
	if current == nil || current.Acme == nil {
		return a
	}

	cur := current.Acme
	a.Enabled = cur.Enabled
	a.Email = strings.TrimSpace(cur.Email)
	if cur.RenewBeforeDays > 0 {
		a.RenewBeforeDays = cur.RenewBeforeDays
	}
	if storage := strings.TrimSpace(cur.Storage); storage != "" {
		a.Storage = storage
	}
	if cur.FallbackSelfSigned != nil {
		a.Fallback = *cur.FallbackSelfSigned
	}
	switch len(cur.Challenge) {
	case 1:
		a.Challenge = cur.Challenge[0]
	default:
		// 多个挑战同时启用等价于 auto；空数组也回到 auto。
		if len(cur.Challenge) > 1 {
			a.Challenge = config.ACMEChallengeAuto
		}
	}

	a.CA, a.Directory, a.EABKid, a.EABHmacKey = acmeCAFromConfig(cur)
	return a
}

// acmeCAFromConfig 把 cas 列表的第一项还原成向导里的 CA 选项。
// 降级链上后面的 CA 不参与向导（向导只写一条），但不会被静默丢弃——
// buildConfig 仍按选择重建整段 cas。
func acmeCAFromConfig(cur *config.AcmeConfig) (acmeCAChoice, string, string, string) {
	if cur.Staging {
		return acmeCAStaging, "", "", ""
	}
	if len(cur.CAs) == 0 {
		return acmeCALetsEncrypt, "", "", ""
	}
	first := cur.CAs[0]
	switch {
	case first.Name == "zerossl" || first.Directory == config.ACMEZeroSSLDirectory:
		return acmeCAZeroSSL, "", first.EABKid, first.EABHmacKey
	case first.Directory == config.ACMELetsEncryptDirectory:
		return acmeCALetsEncrypt, "", "", ""
	default:
		return acmeCACustom, first.Directory, first.EABKid, first.EABHmacKey
	}
}

// overrideACMEAnswers 只应用显式传入的 flag，其余字段保留配置里的值。
func overrideACMEAnswers(a *acmeAnswers, cmd *cobra.Command) {
	flags := cmd.Flags()
	if flags.Changed("email") {
		a.Email, _ = flags.GetString("email")
	}
	if flags.Changed("ca") {
		ca, _ := flags.GetString("ca")
		a.CA = acmeCAChoice(ca)
	}
	if flags.Changed("directory") {
		a.Directory, _ = flags.GetString("directory")
	}
	if flags.Changed("eab-kid") {
		a.EABKid, _ = flags.GetString("eab-kid")
	}
	if flags.Changed("eab-hmac-key") {
		a.EABHmacKey, _ = flags.GetString("eab-hmac-key")
	}
	if flags.Changed("challenge") {
		a.Challenge, _ = flags.GetString("challenge")
	}
	if flags.Changed("renew-before-days") {
		a.RenewBeforeDays, _ = flags.GetInt("renew-before-days")
	}
	if flags.Changed("storage") {
		a.Storage, _ = flags.GetString("storage")
	}
	if flags.Changed("fallback-self-signed") {
		a.Fallback, _ = flags.GetBool("fallback-self-signed")
	}
}

func cmpOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
