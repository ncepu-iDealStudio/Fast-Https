// Package validate 是 check 与 reload 路径的完整校验：主配置、include 展开
// 与逐个 server 块。校验与解析共用 schema 包的结构，避免两套代码对配置
// 结构的理解各自漂移。
package validate

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fast-https/config/internal/acme"
	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/source"
	"fast-https/infra/logging"
)

// CheckConfig check whether config is correct
func CheckConfig() error {
	return ValidateConfigFile(model.CONFIG_FILE_PATH)
}

// ValidateConfigFile validates the main json config file and included json files.
// 告警只写控制台日志；需要拿到告警文本的调用方用 ValidateConfigFileWithWarnings。
func ValidateConfigFile(configPath string) error {
	warnings, err := ValidateConfigFileWithWarnings(configPath)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		logging.ConsoleLog.Warn().Msg(w)
	}
	return nil
}

// ValidateConfigFileWithWarnings 与 ValidateConfigFile 做同样的校验，
// 但把非致命的告警作为返回值交给调用方，不写日志。校验失败时不返回告警。
func ValidateConfigFileWithWarnings(configPath string) ([]string, error) {
	return validateConfigAt(configPath, liveValidateOpts(filepath.Dir(configPath)))
}

// ValidateConfigTree 校验一棵候选目录：<root>/fast-https.json 与其 http.include。
// include 与相对证书路径按 root 解析，不读线上 conf.d；证书若不在候选树里，
// 再查额外的解析基准（线上配置目录）与运行期证书目录。
func ValidateConfigTree(root string, certBases ...string) ([]string, error) {
	abs, err := paths.ResolveCandidateRoot(root)
	if err != nil {
		return nil, err
	}
	main := filepath.Join(abs, "fast-https.json")
	if _, err := os.Stat(main); err != nil {
		return nil, err
	}
	return validateConfigAt(main, validateOpts{
		includeBase: abs,
		treeRoot:    abs,
		certBase:    abs,
		certBases:   certBases,
	})
}

func liveValidateOpts(configDir string) validateOpts {
	return validateOpts{
		includeBase:      model.FAST_HTTPS_ETC_DIR,
		includeFallbacks: []string{"."},
		certBase:         configDir,
	}
}

type validateOpts struct {
	includeBase      string
	includeFallbacks []string
	// treeRoot 非空表示候选树：include 不得逃出该根；证书先查树内再查运行期证书目录。
	treeRoot string
	certBase string
	// certBases 是候选树之外的证书解析基准（线上配置目录）。候选树只复制
	// json、不带证书，相对路径必须按线上目录解析，不能靠进程工作目录——
	// 桌面端从快捷方式启动时 CWD 可能根本不在安装目录。
	certBases []string
	// warn 非 nil 时，非致命的校验问题写进调用方的告警列表而不是报错。
	warn *[]string
}

func validateConfigAt(configPath string, opts validateOpts) ([]string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var root schema.Config
	if err := schema.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("parse config json failed: %w", err)
	}

	var warnings []string
	// 告警统一由这里收集，逐 server 校验只负责记录。
	opts.warn = &warnings

	baseDir := opts.certBase
	if baseDir == "" {
		baseDir = filepath.Dir(configPath)
	}

	if root.WAF != nil {
		switch root.WAF.Mode {
		case "", "enforce", "monitor", "disabled":
		default:
			return nil, fmt.Errorf("waf.mode is invalid: %s (want enforce, monitor or disabled)", root.WAF.Mode)
		}
	}

	if err := acme.Validate(root.Acme); err != nil {
		return nil, err
	}
	if err := model.ValidateSecurity(root.Security); err != nil {
		return nil, err
	}
	if err := validateDefaultCertificate(root.TLS, baseDir, opts); err != nil {
		return nil, err
	}
	if err := validateProxy(root.Proxy); err != nil {
		return nil, err
	}

	serverCount := len(root.HTTP.Servers)
	for i := range root.HTTP.Servers {
		if err := validateServerWith(&root.HTTP.Servers[i], baseDir, baseDir,
			fmt.Sprintf("http.server[%d]", i), true, opts); err != nil {
			return nil, err
		}
	}

	includeServers, err := validateIncludes(root.HTTP.Include, baseDir, opts)
	if err != nil {
		return nil, err
	}
	// 站点允许只写在 include 目录里，主配置不出现 http.server；
	// 但整体一个站点都没有就等于没配服务。
	// 停用文件（*.json.disabled）也是「配置过站点」的证据：把最后一个
	// 站点停用时，候选树里只剩 .json.disabled，此时不应报
	// "no server configured" 而让 site disable 事务失败。
	configured := serverCount + len(includeServers)
	if configured == 0 && len(disabledSiteFiles(root.HTTP.Include, opts)) > 0 {
		configured = 1
	}
	if configured == 0 {
		return nil, errors.New("no server configured: add http.server or a server file under http.include")
	}

	// http-01 需要对外暴露 80 端口，但也可能由前置反代转发，因此只告警不阻断。
	// 判断范围必须包含 include 里的站点：站点通常只写在 conf.d，只看 http.server
	// 会对"80 明明在听"的配置误报，反而把真正的配置问题盖掉。
	allServers := make([]schema.Server, 0, serverCount+len(includeServers))
	allServers = append(allServers, root.HTTP.Servers...)
	allServers = append(allServers, includeServers...)
	if w := acme.HTTP01Port80Warning(root.Acme, allServers); w != "" {
		warnings = append(warnings, w)
	}
	return warnings, nil
}

// validateIncludes 校验 include 目录/文件，返回解析后的 server 块。
// 返回值给 http-01 端口判断用：只看主配置会漏掉写在 conf.d 里的站点。
func validateIncludes(includes []string, rootBaseDir string, opts validateOpts) ([]schema.Server, error) {
	files, err := source.IncludeFiles(includes, opts.includeBase, opts.treeRoot, opts.includeFallbacks...)
	if err != nil {
		return nil, err
	}
	servers := make([]schema.Server, 0, len(files))
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read include file failed: %w", err)
		}

		var s schema.Server
		if err := schema.Unmarshal(content, &s); err != nil {
			return nil, fmt.Errorf("parse include json failed (%s): %w", path, err)
		}

		if err := validateServerWith(&s, filepath.Dir(path), rootBaseDir, path, true, opts); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

// disabledSiteFiles 列出 http.include 目录下的停用站点文件（*.json.disabled）。
// 它们不参与运行、不做 server 校验，但能证明「配置树里配置过站点」——
// 把唯一站点停用时，候选树里没有 .json 了，靠它避免 "no server configured"。
func disabledSiteFiles(includes []string, opts validateOpts) []string {
	var out []string
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			continue
		}
		fullPath := paths.PathIn(opts.includeBase, includePath)
		if opts.treeRoot != "" {
			if err := paths.WithinRoot(opts.treeRoot, fullPath); err != nil {
				continue
			}
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			if strings.HasSuffix(strings.ToLower(fullPath), ".json.disabled") {
				out = append(out, fullPath)
			}
			continue
		}
		_ = filepath.Walk(fullPath, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil || fi.IsDir() {
				return nil
			}
			if opts.treeRoot != "" {
				if err := paths.WithinRoot(opts.treeRoot, path); err != nil {
					return nil
				}
			}
			if strings.HasSuffix(strings.ToLower(path), ".json.disabled") {
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}

// validateDefaultCertificate 校验 tls.default_certificate：要么不配，要么
// 证书与私钥成对出现且文件存在。它不参与 CertInit 自动生成，所以缺失
// 文件是硬错误，不像站点证书那样可能是首次启动前的占位。
func validateDefaultCertificate(tls schema.TLS, baseDir string, opts validateOpts) error {
	cert := strings.TrimSpace(tls.DefaultCertificate.Certificate)
	key := strings.TrimSpace(tls.DefaultCertificate.Key)
	if cert == "" && key == "" {
		return nil
	}
	if cert == "" || key == "" {
		return errors.New("tls.default_certificate: certificate and key must be set together")
	}
	if !pathExistsWith(cert, baseDir, baseDir, opts) {
		return fmt.Errorf("tls.default_certificate.certificate not found: %s", cert)
	}
	if !pathExistsWith(key, baseDir, baseDir, opts) {
		return fmt.Errorf("tls.default_certificate.key not found: %s", key)
	}
	return nil
}

// validateProxy 校验全局代理策略：trusted_proxies 每项必须是 IP 或 CIDR，
// max_hops 不能为负数（0 表示不限制）。
func validateProxy(proxy schema.Proxy) error {
	if proxy.ForwardedFor.MaxHops < 0 {
		return fmt.Errorf("proxy.forwarded_for.max_hops must be >= 0: %d", proxy.ForwardedFor.MaxHops)
	}
	for i, raw := range proxy.TrustedProxies {
		item := strings.TrimSpace(raw)
		if item == "" {
			return fmt.Errorf("proxy.trusted_proxies[%d]: must not be empty", i)
		}
		if _, err := netip.ParsePrefix(item); err == nil {
			continue
		}
		if _, err := netip.ParseAddr(item); err != nil {
			return fmt.Errorf("proxy.trusted_proxies[%d]: invalid IP or CIDR: %q", i, item)
		}
	}
	return nil
}

// validateServer 校验一个 server 块（主配置数组元素或 include 文件）。
func validateServer(s *schema.Server, certBaseDir, rootBaseDir, where string, requireCerts bool) error {
	return validateServerWith(s, certBaseDir, rootBaseDir, where, requireCerts, validateOpts{})
}

func validateServerWith(s *schema.Server, certBaseDir, rootBaseDir, where string, requireCerts bool, opts validateOpts) error {
	if len(s.Listen) == 0 {
		return fmt.Errorf("%s: listen must be non-empty", where)
	}
	// 一个块可以写多个监听，任意一个带 ssl 就要有证书。
	ssl := false
	for _, listen := range s.Listen {
		if err := validateListenPort(listen); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if strings.Contains(listen, "ssl") {
			ssl = true
		}
	}

	if strings.TrimSpace(s.ServerName) == "" {
		return fmt.Errorf("%s: server_name is required", where)
	}

	if ssl {
		// 两个字段要么都写、要么都不写。留空表示交给首次 start 的 CertInit
		// 按站点自动生成——新建 TLS 站点时证书文件本来还不存在，如果强制
		// 要求写路径、又要求文件存在，就永远无法通过校验。
		certEmpty := strings.TrimSpace(s.SSLCertificate) == ""
		keyEmpty := strings.TrimSpace(s.SSLCertificateKey) == ""
		if certEmpty != keyEmpty {
			return fmt.Errorf("%s: ssl_certificate and ssl_certificate_key must be set together", where)
		}
		// 显式写了路径就要求文件存在（requireCerts 控制，落盘新建时关闭）。
		// 报错里补一句怎么办：装完包直接 start 是新用户最常见的卡点。
		//
		// 候选树是例外：它按设计只复制 json、不带证书文件，而且首次 start
		// 才会用本地 CA 生成自签证书。在这里硬拒会让「装完包还没启动过」
		// 和「新增 TLS 站点」永远卡在校验上，所以降级成告警。
		if requireCerts && !certEmpty && !pathExistsWith(s.SSLCertificate, certBaseDir, rootBaseDir, opts) {
			msg := fmt.Sprintf("%s: ssl_certificate not found: %s "+
				"(首次 start 会用本地 CA 生成自签证书，先跑一次 fast-https start；或用 acme 申请后指向生成的证书)",
				where, s.SSLCertificate)
			if !warnOnly(opts, msg, s.ServerName, s.SSLCertificate) {
				return fmt.Errorf("%s", msg)
			}
		}
		if requireCerts && !keyEmpty && !pathExistsWith(s.SSLCertificateKey, certBaseDir, rootBaseDir, opts) {
			msg := fmt.Sprintf("%s: ssl_certificate_key not found: %s "+
				"(首次 start 会用本地 CA 生成自签证书)", where, s.SSLCertificateKey)
			if !warnOnly(opts, msg, s.ServerName, s.SSLCertificateKey) {
				return fmt.Errorf("%s", msg)
			}
		}
	}

	if len(s.Locations) == 0 {
		return fmt.Errorf("%s: location must contain at least one rule", where)
	}

	for i := range s.Locations {
		loc := &s.Locations[i]
		if strings.TrimSpace(loc.URL) == "" {
			return fmt.Errorf("%s: location[%d].url is required", where, i)
		}

		typeStr := loc.Type
		if typeStr == "" {
			typeStr = "local"
		}

		switch typeStr {
		case "local":
			if strings.TrimSpace(loc.Root) == "" {
				return fmt.Errorf("%s: location[%d].root is required for local type", where, i)
			}
			if err := validateSymlinkPolicy(loc.SymlinkPolicy, loc.Root, fmt.Sprintf("%s: location[%d]", where, i)); err != nil {
				return err
			}
		case "proxy":
			if strings.TrimSpace(loc.SymlinkPolicy) != "" {
				return fmt.Errorf("%s: location[%d].symlink_policy only applies to local type", where, i)
			}
			pass := strings.TrimSpace(loc.ProxyPass)
			if pass == "" {
				return fmt.Errorf("%s: location[%d].proxy_pass is required for proxy type", where, i)
			}
			if !strings.HasPrefix(pass, "http://") && !strings.HasPrefix(pass, "https://") {
				return fmt.Errorf("%s: location[%d].proxy_pass must start with http:// or https://", where, i)
			}
		case "rewrite", "devmod":
			// valid and no extra required fields for minimal validator
		default:
			return fmt.Errorf("%s: location[%d].type is invalid: %s", where, i, typeStr)
		}

		if err := validateLocationAuth(&loc.Auth, where, i); err != nil {
			return err
		}

		for j, method := range loc.AllowMethods {
			if strings.TrimSpace(method) == "" {
				return fmt.Errorf("%s: location[%d].allow_methods[%d] is empty", where, i, j)
			}
			if strings.ContainsAny(method, " \t") {
				return fmt.Errorf("%s: location[%d].allow_methods[%d] must be a single method: %q", where, i, j, method)
			}
		}
	}

	return nil
}

// validateLocationAuth 校验 location 的 auth 段：只支持 basic，凭据必须写全。
// 早先只写 type 而漏掉 user/pswd 时，"Basic Og=="（即 ":"）就能通过，
// 等于开了个空口令的门。
func validateLocationAuth(auth *model.PathAuth, where string, index int) error {
	authType := strings.TrimSpace(auth.AuthType)
	if authType == "" {
		return nil
	}
	if !strings.EqualFold(authType, "basic") {
		return fmt.Errorf("%s: location[%d].auth.type must be basic: %q", where, index, auth.AuthType)
	}
	if strings.TrimSpace(auth.User) == "" {
		return fmt.Errorf("%s: location[%d].auth.user is required when auth.type is set", where, index)
	}
	if auth.Pswd == "" {
		return fmt.Errorf("%s: location[%d].auth.pswd is required when auth.type is set", where, index)
	}
	// realm 会原样写进响应头，引号与换行会把头拆坏。
	if strings.ContainsAny(auth.Realm, "\"\r\n") {
		return fmt.Errorf("%s: location[%d].auth.realm must not contain quotes or line breaks", where, index)
	}
	return nil
}

func validateListenPort(listen string) error {
	parts := strings.Fields(strings.TrimSpace(listen))
	if len(parts) == 0 {
		return errors.New("listen must be non-empty")
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid listen port: %s", parts[0])
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("listen port out of range: %d", port)
	}
	return nil
}

// warnOnly 在「候选树校验」里把证书缺失降级为告警并返回 true；线上校验
// 或自定义证书路径返回 false，调用方照旧报错。
//
// 只放过 <主域名>.pem / <主域名>-key.pem 这种约定名：它们是 CertInit 在首次
// start 时自动生成的位置，候选树按设计又不带证书文件。自定义路径（比如
// "no-such-cert.pem"）缺失几乎一定是写错了，仍然要拦。
func warnOnly(opts validateOpts, msg, serverName, certPath string) bool {
	if opts.treeRoot == "" || opts.warn == nil || !certAutoGenerated(certPath, serverName) {
		return false
	}
	*opts.warn = append(*opts.warn, msg+"（候选树不含证书文件，首次 start 会生成）")
	return true
}

// certAutoGenerated 判断证书路径是不是「首次 start 由 CertInit 生成」的约定位置。
func certAutoGenerated(certPath, serverName string) bool {
	names := strings.Fields(serverName)
	if len(names) == 0 {
		return false
	}
	primary := strings.ToLower(names[0])
	base := strings.ToLower(filepath.Base(strings.TrimSpace(certPath)))
	return base != "" && (base == primary+".pem" || base == primary+"-key.pem")
}

func pathExists(path, baseDir, rootBaseDir string) bool {
	return pathExistsWith(path, baseDir, rootBaseDir, validateOpts{})
}

func pathExistsWith(path, baseDir, rootBaseDir string, opts validateOpts) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if opts.treeRoot != "" {
		return candidateCertExists(path, baseDir, opts.treeRoot, opts.certBases)
	}
	// Certificate paths in the main config are relative to the ETC base
	// (./config for archives, /etc/fast-https for packages), which is also how
	// tls.LoadX509KeyPair resolves them after ResolveRuntimePaths. Paths inside
	// an included file may instead be relative to that file or the main config.
	candidates := []string{path, paths.ETCPath(path), paths.DataPath(path)}
	if !filepath.IsAbs(path) {
		candidates = append(candidates,
			filepath.Join(baseDir, path),
			filepath.Join(rootBaseDir, path),
		)
	}
	return anyFileExists(candidates)
}

func candidateCertExists(path, fileDir, treeRoot string, extraBases []string) bool {
	var candidates []string
	if filepath.IsAbs(path) {
		candidates = append(candidates, path)
	} else {
		candidates = append(candidates,
			paths.PathIn(treeRoot, path),
			filepath.Join(fileDir, path),
		)
		// 候选树只带 json，证书文件留在线上树：按调用方给的线上配置目录解析。
		// 这样解析结果与进程 CWD 无关（桌面端可能从任意工作目录启动）。
		for _, base := range extraBases {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, path),
				paths.PathIn(base, path),
			)
		}
	}
	if anyFileExists(candidates) {
		return true
	}
	// 候选树没有时，按绝对路径（已试过）或运行期证书目录再查一次，不走 "." 兜底。
	return anyFileExists([]string{
		paths.PathIn(model.FAST_HTTPS_ETC_DIR, path),
		paths.PathIn(model.CERT_DIR, path),
	})
}

func anyFileExists(candidates []string) bool {
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}
