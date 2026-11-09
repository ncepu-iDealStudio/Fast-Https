package cmd

import (
	"errors"
	"fast-https/config"
	"fast-https/modules/site"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// site 命令组管理 include 目录下的站点文件（conf.d/<name>.json）。
// 站点文件与主配置 http.server[] 的元素同构，合并与校验复用 config 包；
// 写完只提示 reload，不自动重载。

type siteOptions struct {
	ConfigPath     string
	Name           string
	Listens        []string
	ServerName     string
	Root           string
	ProxyPass      string
	LocationType   string
	TLS            bool
	Force          bool
	Yes            bool
	Merged         bool
	NonInteractive bool
	// Plain 走逐行问答；Windows 控制台默认启用，见 runSiteForm。
	Plain bool
	// Changed 报告某个 flag 是否被显式传入。同名站点按"更新"处理时，
	// 只有显式传入的项才覆盖现有值——全部按默认值写回会把上次的配置抹掉。
	Changed func(string) bool
	In      io.Reader
	Out     io.Writer
}

// siteFlagChanged 是 Changed 的 nil 安全包装：测试里直接构造 siteOptions 时没有它。
func siteFlagChanged(opts siteOptions, name string) bool {
	return opts.Changed != nil && opts.Changed(name)
}

func newSiteCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "site",
		Short: t("管理 http.include 目录下的站点文件", "manage site files under http.include"),
		Long: t(
			"新增、更新、删除、启用、停用用 site add/update/delete/enable/disable，它们走配置事务（校验、恢复点、reload、可回滚）。site add 不带 --domain 且标准输入是终端时进向导。",
			"Use site add/update/delete/enable/disable for adding, updating, deleting, enabling and disabling; they go through the config transaction (validation, restore point, reload, rollback). site add enters a wizard when --domain is omitted on a terminal."),
		SilenceUsage: true,
		RunE:         func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}

	listCmd := &cobra.Command{
		Use:          "list",
		Short:        t("列出站点文件的 listen、server_name 与 location 类型", "list site files with listen, server_name and location types"),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonMode(cmd) {
				return runSiteListJSON(cmd)
			}
			return runSiteList(siteOptionsFrom(cmd, args))
		},
	}
	showCmd := &cobra.Command{
		Use:          "show <name>",
		Short:        t("打印某站点最终生效的 server 块及其来源文件", "print the effective server block of a site and its source file"),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonMode(cmd) {
				return runSiteShowJSON(cmd, args[0])
			}
			return runSiteShow(siteOptionsFrom(cmd, args))
		},
	}
	for _, sub := range []*cobra.Command{listCmd, showCmd} {
		sub.Flags().String("config", config.CONFIG_FILE_PATH, t("主配置文件", "main config file"))
	}
	showCmd.Flags().Bool("merged", false, t("打印合并后每个字段都生效的 server 块", "print the merged server block with every effective field"))

	root.AddCommand(listCmd, showCmd)
	addSiteWriteCommands(root)
	return root
}

// siteOptionsFrom 读取公共 flag 与位置参数（站点名）。
func siteOptionsFrom(cmd *cobra.Command, args []string) siteOptions {
	flags := cmd.Flags()
	path, _ := flags.GetString("config")
	plain, _ := flags.GetBool("plain")
	opts := siteOptions{
		Plain:      plain,
		ConfigPath: path,
		Changed:    flags.Changed,
		In:         cmd.InOrStdin(),
		Out:        cmd.OutOrStdout(),
	}
	if len(args) > 0 {
		// 名字就是文件名主干：带 .json 也认，避免生成 a.json.json。
		opts.Name = strings.TrimSuffix(strings.TrimSpace(args[0]), ".json")
	}
	if v, err := flags.GetStringArray("listen"); err == nil {
		opts.Listens = v
	}
	if v, err := flags.GetString("server-name"); err == nil {
		opts.ServerName = v
	}
	if v, err := flags.GetString("root"); err == nil {
		opts.Root = v
	}
	if v, err := flags.GetString("proxy-pass"); err == nil {
		opts.ProxyPass = v
	}
	if v, err := flags.GetString("type"); err == nil {
		opts.LocationType = v
	}
	if v, err := flags.GetBool("tls"); err == nil {
		opts.TLS = v
	}
	if v, err := flags.GetBool("force"); err == nil {
		opts.Force = v
	}
	if v, err := flags.GetBool("yes"); err == nil {
		opts.Yes = v
	}
	if v, err := flags.GetBool("merged"); err == nil {
		opts.Merged = v
	}
	if v, err := flags.GetBool("non-interactive"); err == nil {
		opts.NonInteractive = v
	}
	// 标准输入不是终端时无法提问，直接走参数模式。
	opts.NonInteractive = opts.NonInteractive || !term.IsTerminal(os.Stdin.Fd())
	return opts
}

// siteDirs 读取主配置里的 include 目录，空时给出可执行的提示。
// 用 --config 指定的文件而不是进程快照：site 命令不启动服务，也不该改全局状态。
func siteDirs(opts siteOptions) ([]string, error) {
	dirs, err := config.SiteDirsFromFile(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("主配置 %s 没有可用的 http.include 目录，请先配置 http.include", opts.ConfigPath)
	}
	return dirs, nil
}

// applySiteLocation 只改 URL 为 "/" 的那个 location（没有就补一个），
// 用户手写的其它 location 原样保留。
func applySiteLocation(in *config.SiteInput, opts siteOptions, changed func(string) bool) error {
	typeChanged := changed("type")
	rootChanged := changed("root")
	proxyChanged := changed("proxy-pass")
	if !typeChanged && !rootChanged && !proxyChanged {
		return nil
	}

	kind := strings.TrimSpace(opts.LocationType)
	if typeChanged && kind != "local" && kind != "proxy" {
		return fmt.Errorf("location 类型只支持 local 或 proxy：%q", kind)
	}

	loc := primaryLocation(in)
	if loc == nil {
		in.Locations = append(in.Locations, config.SiteLocation{URL: "/"})
		loc = &in.Locations[len(in.Locations)-1]
	}
	if !typeChanged {
		kind = strings.TrimSpace(loc.Type)
		if kind == "" {
			kind = "local"
		}
	}

	switch kind {
	case "proxy":
		upstream := strings.TrimSpace(opts.ProxyPass)
		if !proxyChanged {
			upstream = loc.ProxyPass
		}
		if upstream == "" {
			return errors.New("反代站点需要 --proxy-pass，例如 http://127.0.0.1:8080")
		}
		loc.Type = "proxy"
		loc.ProxyPass = upstream
		loc.Root = ""
		loc.Index = nil
	default:
		loc.Type = "local"
		loc.ProxyPass = ""
		if rootChanged || strings.TrimSpace(loc.Root) == "" {
			root := strings.TrimSpace(opts.Root)
			if root == "" {
				root = "./httpdoc/root"
			}
			loc.Root = root
		}
		if len(loc.Index) == 0 {
			loc.Index = []string{"index.html", "index.htm"}
		}
	}
	return nil
}

func primaryLocation(in *config.SiteInput) *config.SiteLocation {
	for i := range in.Locations {
		if url := strings.TrimSpace(in.Locations[i].URL); url == "/" || url == "" {
			return &in.Locations[i]
		}
	}
	return nil
}

// fillSiteCerts 在监听带 ssl 时补默认证书路径，与 init 的规则保持一致。
func fillSiteCerts(in *config.SiteInput) {
	if !listensHaveSSL(in.Listens) {
		return
	}
	name := strings.TrimSpace(in.ServerName)
	if in.SSLCertificate == "" {
		in.SSLCertificate = filepath.Join(config.CERT_DIR, name+".pem")
	}
	if in.SSLCertificateKey == "" {
		in.SSLCertificateKey = filepath.Join(config.CERT_DIR, name+"-key.pem")
	}
}

// siteInputFrom 先用参数拼装；参数不齐且能交互时走向导补齐。
// 返回站点名与结构：向导里可能改掉名字，所以不能只改 opts。
func siteInputFrom(opts *siteOptions) (string, config.SiteInput, error) {
	in := config.SiteInput{
		Listens:    cleanListens(opts.Listens),
		ServerName: strings.TrimSpace(opts.ServerName),
	}
	if in.ServerName == "" {
		in.ServerName = strings.TrimSpace(opts.Name)
	}
	if opts.LocationType == "" {
		opts.LocationType = "local"
	}

	needsName := strings.TrimSpace(opts.Name) == ""
	needsUpstream := opts.LocationType == "proxy" && strings.TrimSpace(opts.ProxyPass) == ""
	if !opts.NonInteractive && (needsName || needsUpstream) {
		if err := promptSite(opts, &in); err != nil {
			return "", config.SiteInput{}, err
		}
	}

	built, err := buildSiteInput(*opts, in)
	if err != nil {
		return "", config.SiteInput{}, err
	}
	return strings.TrimSpace(opts.Name), built, nil
}

// cleanListens 去掉空白与空项，保持书写顺序。
func cleanListens(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if value := strings.TrimSpace(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// splitListens 把向导里的一行文本拆成监听列表（逗号分隔，
// 单条监听内部本来就带空格，例如 "443 ssl h2"）。
func splitListens(text string) []string {
	return cleanListens(strings.Split(text, ","))
}

func listensHaveSSL(listens []string) bool {
	for _, listen := range listens {
		if strings.Contains(listen, "ssl") {
			return true
		}
	}
	return false
}

// listenPort 取监听串里的端口部分，与运行期解析方式一致。
func listenPort(listen string) string {
	fields := strings.Fields(listen)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], ":")
}

// withTLS 给一条监听补上 ssl h2。
func withTLS(listen string) string {
	if !strings.Contains(listen, "ssl") {
		listen += " ssl"
	}
	if !strings.Contains(listen, "h2") && !strings.Contains(listen, "tcp") {
		listen += " h2"
	}
	return listen
}

// resolveListens 决定最终写进站点文件的监听列表。
//
// --tls 的语义是"至少要有一个 TLS 监听"，并且永远不往 80 上贴 ssl
// （那会得到一个既不是 HTTP 也不是 HTTPS 的端口）：
//   - 没给 --listen：不开 TLS 用 80；--tls 用 80 + "443 ssl h2"。
//   - 给了 --listen：写什么用什么。
//   - 给了 --listen 又加 --tls：已有 ssl 监听就原样；否则 443 那条升级成 TLS；
//     没有 443 时把非 80 的监听升级成 TLS；全是 80 就补一条 "443 ssl h2"。
//
// 例：--listen 80 --tls → 80 与 "443 ssl h2"；--listen 8443 --tls →
// "8443 ssl h2"；--listen 8443 --listen 80 --tls → "8443 ssl h2" 与 80。
func resolveListens(opts siteOptions, given []string) []string {
	listens := cleanListens(given)
	if len(listens) == 0 {
		if opts.TLS {
			return []string{"80", "443 ssl h2"}
		}
		return []string{"80"}
	}
	if !opts.TLS || listensHaveSSL(listens) {
		return listens
	}

	for i, listen := range listens {
		if listenPort(listen) == "443" {
			listens[i] = withTLS(listen)
			return listens
		}
	}

	upgraded := false
	for i, listen := range listens {
		if listenPort(listen) == "80" {
			continue
		}
		listens[i] = withTLS(listen)
		upgraded = true
	}
	if !upgraded {
		listens = append(listens, "443 ssl h2")
	}
	return listens
}

// buildSiteInput 把选项落成站点结构：监听与证书由 TLS 规则决定，
// 类型决定生成 local 还是 proxy 的 location。
func buildSiteInput(opts siteOptions, in config.SiteInput) (config.SiteInput, error) {
	locationType := strings.TrimSpace(opts.LocationType)
	if locationType == "" {
		locationType = "local"
	}
	if locationType != "local" && locationType != "proxy" {
		return config.SiteInput{}, fmt.Errorf("location 类型只支持 local 或 proxy：%q", locationType)
	}

	in.Listens = resolveListens(opts, in.Listens)
	tls := listensHaveSSL(in.Listens)
	in.ServerName = strings.TrimSpace(in.ServerName)
	if in.ServerName == "" {
		return config.SiteInput{}, errors.New("server_name 不能为空")
	}

	if tls {
		if in.SSLCertificate == "" {
			in.SSLCertificate = filepath.Join(config.CERT_DIR, in.ServerName+".pem")
		}
		if in.SSLCertificateKey == "" {
			in.SSLCertificateKey = filepath.Join(config.CERT_DIR, in.ServerName+"-key.pem")
		}
	}

	switch locationType {
	case "proxy":
		upstream := strings.TrimSpace(opts.ProxyPass)
		if upstream == "" {
			return config.SiteInput{}, errors.New("反代站点需要 --proxy-pass，例如 http://127.0.0.1:8080")
		}
		in.Locations = []config.SiteLocation{{URL: "/", Type: "proxy", ProxyPass: upstream}}
	default:
		root := strings.TrimSpace(opts.Root)
		if root == "" {
			root = "./httpdoc/root"
		}
		in.Locations = []config.SiteLocation{{
			URL:   "/",
			Type:  "local",
			Root:  root,
			Index: []string{"index.html", "index.htm"},
		}}
	}
	return in, nil
}

// promptSitePlain 用纯逐行问答走完整条向导。
// Windows 的 conhost 对 bubbletea 的终端控制支持不完整（raw 读、全屏渲染
// 都会返回 syscall.EWINDOWS，也就是 "not supported by windows"），因此
// Windows 上完全不碰 huh 与 term，只读 stdin。
func promptSitePlain(opts *siteOptions, in *config.SiteInput) error {
	// 逐行模式的标题与说明刻意和 huh 表单保持一字不差：同一套向导，
	// 只是终端不支持全屏渲染时退化成一行一问。
	ask := func(title, desc, initial string) (string, error) {
		if desc != "" {
			fmt.Fprintln(opts.Out, desc)
		}
		return readPlainPathLine(*opts, title, initial)
	}

	if strings.TrimSpace(opts.Name) == "" {
		name, err := ask("站点名（文件名）",
			"生成 conf.d/<名字>.json，只能用字母、数字、点、短横线。", "")
		if err != nil {
			return err
		}
		if err := site.ValidateName(strings.TrimSpace(name)); err != nil {
			return err
		}
		opts.Name = strings.TrimSpace(name)
	}

	listen, err := ask("监听端口/参数",
		`一个或多个，逗号分隔：80, "443 ssl h2"。`, strings.Join(in.Listens, ", "))
	if err != nil {
		return err
	}
	if strings.TrimSpace(listen) != "" {
		in.Listens = splitListens(listen)
	}

	serverName, err := ask("server_name",
		"按请求 Host 匹配的域名，留空则与站点名相同。", in.ServerName)
	if err != nil {
		return err
	}
	in.ServerName = strings.TrimSpace(serverName)
	if in.ServerName == "" {
		in.ServerName = strings.TrimSpace(opts.Name)
	}

	kind, err := ask("站点类型",
		"1. 静态站点（本地目录）\n2. 反向代理（转发到上游）\nEnter a number between 1 and 2:", "1")
	if err != nil {
		return err
	}
	if strings.TrimSpace(kind) == "2" {
		opts.LocationType = "proxy"
	} else {
		opts.LocationType = "local"
	}

	tls, err := ask("启用 TLS？",
		"会补 ssl h2 到 listen，并给出默认证书路径。", "n")
	if err != nil {
		return err
	}
	opts.TLS = strings.HasPrefix(strings.ToLower(strings.TrimSpace(tls)), "y")

	if opts.LocationType == "proxy" {
		upstream, err := ask("上游地址",
			"反代目标，例如 http://127.0.0.1:8080。", "http://127.0.0.1:8080")
		if err != nil {
			return err
		}
		opts.ProxyPass = strings.TrimSpace(upstream)
		return nil
	}

	root, err := ask("站点根目录",
		"静态文件目录，相对路径按站点根基址解析。", "./httpdoc/root")
	if err != nil {
		return err
	}
	opts.Root = strings.TrimSpace(root)
	return nil
}

// supportsTUI 判断当前终端能否跑 bubbletea 的全屏表单。
// Windows 的 conhost 不行：raw 读取与全屏渲染都会返回 syscall.EWINDOWS
// （也就是 not supported by windows）。Windows Terminal 等现代终端可以。
func supportsTUI() bool {
	if runtime.GOOS != "windows" {
		return true
	}
	return os.Getenv("WT_SESSION") != "" || os.Getenv("TERM_PROGRAM") != ""
}

// promptSite 只问缺什么：名字、类型、上游，以及 TLS 开关。
func promptSite(opts *siteOptions, in *config.SiteInput) error {
	// 终端支持全屏渲染才用 huh 表单（与 site init 时代一致）；conhost 之类
	// 不支持的终端退化成逐行问答，标题与说明文字保持一字不差。
	if opts.Plain || !supportsTUI() {
		return promptSitePlain(opts, in)
	}
	types := []huh.Option[string]{
		huh.NewOption("静态站点（本地目录）", "local"),
		huh.NewOption("反向代理（转发到上游）", "proxy"),
	}

	// 一题一屏：每个字段单独成组，答完回车进入下一题。
	listenText := strings.Join(in.Listens, ", ")
	groups := make([]*huh.Group, 0, 5)
	// 同名更新时文件名是确定的，不再问一遍。
	if strings.TrimSpace(opts.Name) == "" {
		groups = append(groups, huh.NewGroup(huh.NewInput().
			Title("站点名（文件名）").
			Description("生成 conf.d/<名字>.json，只能用字母、数字、点、短横线。").
			Placeholder("blog").
			Value(&opts.Name).
			Validate(site.ValidateName)))
	}
	groups = append(groups,
		huh.NewGroup(huh.NewInput().
			Title("监听端口/参数").
			Description(`一个或多个，逗号分隔：80, "443 ssl h2"。`).
			Value(&listenText)),
		huh.NewGroup(huh.NewInput().
			Title("server_name").
			Description("按请求 Host 匹配的域名，留空则与站点名相同。").
			Value(&in.ServerName)),
		huh.NewGroup(huh.NewSelect[string]().
			Title("站点类型").
			Options(types...).
			Value(&opts.LocationType)),
		huh.NewGroup(huh.NewConfirm().
			Title("启用 TLS？").
			Description("会补 ssl h2 到 listen，并给出默认证书路径。").
			Affirmative("是").
			Negative("否").
			Value(&opts.TLS)),
	)
	first := huh.NewForm(groups...)

	if err := runSiteForm(first, opts); err != nil {
		return err
	}

	in.Listens = splitListens(listenText)
	if strings.TrimSpace(in.ServerName) == "" {
		in.ServerName = strings.TrimSpace(opts.Name)
	}

	// 第二屏必须等第一屏的"站点类型"定下来再构造：类型是在向导里选的，
	// 提前按默认值（local）构造就会在选了反向代理之后继续问"站点根目录"，
	// 结果 ProxyPass 永远为空，最后只能报 "反代站点需要 --proxy-pass"。
	if opts.LocationType == "proxy" {
		return runSiteForm(huh.NewForm(huh.NewGroup(huh.NewInput().
			Title("上游地址").
			Description("反代目标，例如 http://127.0.0.1:8080。").
			Placeholder("http://127.0.0.1:8080").
			Value(&opts.ProxyPass).
			Validate(validateNotBlank))), opts)
	}

	initial := strings.TrimSpace(opts.Root)
	if initial == "" {
		initial = "./httpdoc/root"
	}
	if _, ok := canReadRaw(*opts); !ok {
		// 非终端输入时没有 Tab 可用，退回普通输入框。
		return runSiteForm(huh.NewForm(huh.NewGroup(huh.NewInput().
			Title("站点根目录").
			Description("静态文件目录，相对路径按站点根基址解析。").
			Placeholder(initial).
			Value(&opts.Root))), opts)
	}

	// 路径靠手打容易错，这里给 shell 式的 Tab 补全。
	root, err := readPathLine(*opts, "站点根目录", initial)
	if err != nil {
		return err
	}
	opts.Root = root
	return nil
}

// runSiteForm 跑一屏问答，把 Ctrl+C / Esc 统一翻译成"已取消"。
func runSiteForm(form *huh.Form, opts *siteOptions) error {
	// Windows 的 conhost 对 bubbletea 全屏渲染支持不稳定，会直接抛
	// "not supported by windows"；逐行问答在哪个终端都能跑，所以默认降级。
	if opts.Plain || runtime.GOOS == "windows" {
		form = form.WithAccessible(true)
	}
	applyStdio(form, opts.In, opts.Out, false)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("已取消，未生成站点文件")
		}
		return err
	}
	return nil
}

func runSiteList(opts siteOptions) error {
	dirs, err := siteDirs(opts)
	if err != nil {
		return err
	}
	entries, err := site.List(dirs)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintf(opts.Out, "%s 下没有站点文件。用 fast-https site add 新建。\n", strings.Join(dirs, ", "))
		return nil
	}
	fmt.Fprintf(opts.Out, "%-16s %-14s %-28s %s\n", "NAME", "LISTEN", "SERVER_NAME", "LOCATION")
	for _, entry := range entries {
		if entry.Invalid != "" {
			fmt.Fprintf(opts.Out, "%-16s 解析失败：%s\n", entry.Name, entry.Invalid)
			continue
		}
		fmt.Fprintf(opts.Out, "%-16s %-14s %-28s %s\n", entry.Name, entry.Listen, entry.ServerName,
			strings.Join(entry.Types, ", "))
	}
	return nil
}

// runSiteShow 默认打印站点文件原文，--merged 才打印合并后的完整 server 块：
// 后者把未写字段的默认值也铺开，排错时有用，日常查看太吵。
func runSiteShow(opts siteOptions) error {
	if !opts.Merged {
		if dirs, err := siteDirs(opts); err == nil {
			if path, ok := site.Find(dirs, opts.Name); ok {
				raw, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}
				fmt.Fprintf(opts.Out, "// 来源：%s（原文；加 --merged 看合并后的生效字段）\n", path)
				fmt.Fprint(opts.Out, string(raw))
				return nil
			}
		}
	}

	content, source, err := config.SnapshotServer(opts.ConfigPath, opts.Name)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "// 来源：%s\n", source)
	fmt.Fprint(opts.Out, string(content))
	return nil
}
