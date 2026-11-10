// check 与 reload 路径的完整校验：主配置、include 展开与逐个 server 块。
// 校验与解析共用同一份 schema（jsonConfig/jsonServer），
// 避免两套代码对配置结构的理解各自漂移。
package config

import (
	"errors"
	"fast-https/infra/logging"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CheckConfig check whether config is correct
func CheckConfig() error {
	return ValidateConfigFile(CONFIG_FILE_PATH)
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
// 再查引用的绝对路径或运行期证书目录。
func ValidateConfigTree(root string) ([]string, error) {
	abs, err := ResolveCandidateRoot(root)
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
	})
}

func liveValidateOpts(configDir string) validateOpts {
	return validateOpts{
		includeBase:      FAST_HTTPS_ETC_DIR,
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
}

func validateConfigAt(configPath string, opts validateOpts) ([]string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}

	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return nil, fmt.Errorf("parse config json failed: %w", err)
	}

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

	if err := root.Acme.validate(); err != nil {
		return nil, err
	}
	if err := root.Security.validate(); err != nil {
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
	if serverCount+len(includeServers) == 0 {
		return nil, errors.New("no server configured: add http.server or a server file under http.include")
	}

	// http-01 需要对外暴露 80 端口，但也可能由前置反代转发，因此只告警不阻断。
	// 判断范围必须包含 include 里的站点：站点通常只写在 conf.d，只看 http.server
	// 会对"80 明明在听"的配置误报，反而把真正的配置问题盖掉。
	allServers := make([]jsonServer, 0, serverCount+len(includeServers))
	allServers = append(allServers, root.HTTP.Servers...)
	allServers = append(allServers, includeServers...)
	var warnings []string
	if w := root.Acme.http01Port80Warning(allServers); w != "" {
		warnings = append(warnings, w)
	}
	return warnings, nil
}

// validateIncludes 校验 include 目录/文件，返回解析后的 server 块。
// 返回值给 http-01 端口判断用：只看主配置会漏掉写在 conf.d 里的站点。
func validateIncludes(includes []string, rootBaseDir string, opts validateOpts) ([]jsonServer, error) {
	files, err := includeFiles(includes, opts.includeBase, opts.treeRoot, opts.includeFallbacks...)
	if err != nil {
		return nil, err
	}
	servers := make([]jsonServer, 0, len(files))
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read include file failed: %w", err)
		}

		var s jsonServer
		if err := unmarshalConfig(content, &s); err != nil {
			return nil, fmt.Errorf("parse include json failed (%s): %w", path, err)
		}

		if err := validateServerWith(&s, filepath.Dir(path), rootBaseDir, path, true, opts); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

// includeFiles 把 http.include 展开成具体的 .json 文件列表：目录递归收集，
// 单个文件必须带 .json 后缀。baseDir 是解析相对路径的基准；treeRoot 非空时
// 拒绝逃出该根（含符号链接）。fallbacks 传给 absPathIn，线上入口传 "."。
func includeFiles(includes []string, baseDir, treeRoot string, fallbacks ...string) ([]string, error) {
	var files []string
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			return nil, errors.New("http.include must contain non-empty paths")
		}

		fullPath := absPathIn(baseDir, includePath, fallbacks...)
		if treeRoot != "" {
			if err := withinRoot(treeRoot, fullPath); err != nil {
				return nil, err
			}
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("include path not found: %s", fullPath)
		}

		if !info.IsDir() {
			if filepath.Ext(fullPath) != ".json" {
				return nil, fmt.Errorf("include file must be .json: %s", fullPath)
			}
			files = append(files, fullPath)
			continue
		}

		err = filepath.Walk(fullPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			if treeRoot != "" {
				if err := withinRoot(treeRoot, path); err != nil {
					return err
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// validateServer 校验一个 server 块（主配置数组元素或 include 文件）。
func validateServer(s *jsonServer, certBaseDir, rootBaseDir, where string, requireCerts bool) error {
	return validateServerWith(s, certBaseDir, rootBaseDir, where, requireCerts, validateOpts{})
}

func validateServerWith(s *jsonServer, certBaseDir, rootBaseDir, where string, requireCerts bool, opts validateOpts) error {
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
		if strings.TrimSpace(s.SSLCertificate) == "" || strings.TrimSpace(s.SSLCertificateKey) == "" {
			return fmt.Errorf("%s: ssl listen requires ssl_certificate and ssl_certificate_key", where)
		}
		// 新建站点时证书由首次 start 生成，落盘那一步不要求文件已经存在。
		// 报错里补一句怎么办：装完包直接 start 是新用户最常见的卡点。
		if requireCerts && !pathExistsWith(s.SSLCertificate, certBaseDir, rootBaseDir, opts) {
			return fmt.Errorf("%s: ssl_certificate not found: %s "+
				"(首次 start 会用本地 CA 生成自签证书，先跑一次 fast-https start；或用 acme 申请后指向生成的证书)",
				where, s.SSLCertificate)
		}
		if requireCerts && !pathExistsWith(s.SSLCertificateKey, certBaseDir, rootBaseDir, opts) {
			return fmt.Errorf("%s: ssl_certificate_key not found: %s "+
				"(首次 start 会用本地 CA 生成自签证书，先跑一次 fast-https start)", where, s.SSLCertificateKey)
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
		case "proxy":
			if len(loc.ProxyPass) == 0 {
				return fmt.Errorf("%s: location[%d].proxy_pass is required for proxy type", where, i)
			}
			var scheme string
			for j, pass := range loc.ProxyPass {
				pass = strings.TrimSpace(pass)
				if pass == "" {
					return fmt.Errorf("%s: location[%d].proxy_pass[%d] is empty", where, i, j)
				}
				if !strings.HasPrefix(pass, "http://") && !strings.HasPrefix(pass, "https://") {
					return fmt.Errorf("%s: location[%d].proxy_pass must start with http:// or https://", where, i)
				}
				this := "http"
				if strings.HasPrefix(pass, "https://") {
					this = "https"
				}
				if scheme == "" {
					scheme = this
				} else if scheme != this {
					return fmt.Errorf("%s: location[%d].proxy_pass mixes http and https", where, i)
				}
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
func validateLocationAuth(auth *PathAuth, where string, index int) error {
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

func pathExists(path, baseDir, rootBaseDir string) bool {
	return pathExistsWith(path, baseDir, rootBaseDir, validateOpts{})
}

func pathExistsWith(path, baseDir, rootBaseDir string, opts validateOpts) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if opts.treeRoot != "" {
		return candidateCertExists(path, baseDir, opts.treeRoot)
	}
	// Certificate paths in the main config are relative to the ETC base
	// (./config for archives, /etc/fast-https for packages), which is also how
	// tls.LoadX509KeyPair resolves them after ResolveRuntimePaths. Paths inside
	// an included file may instead be relative to that file or the main config.
	candidates := []string{path, absETCPath(path), absDataPath(path)}
	if !filepath.IsAbs(path) {
		candidates = append(candidates,
			filepath.Join(baseDir, path),
			filepath.Join(rootBaseDir, path),
		)
	}
	return anyFileExists(candidates)
}

func candidateCertExists(path, fileDir, treeRoot string) bool {
	var candidates []string
	if filepath.IsAbs(path) {
		candidates = append(candidates, path)
	} else {
		candidates = append(candidates,
			absPathIn(treeRoot, path),
			filepath.Join(fileDir, path),
		)
	}
	if anyFileExists(candidates) {
		return true
	}
	// 候选树没有时，按绝对路径（已试过）或运行期证书目录再查一次，不走 "." 兜底。
	return anyFileExists([]string{
		absPathIn(FAST_HTTPS_ETC_DIR, path),
		absPathIn(CERT_DIR, path),
	})
}

func anyFileExists(paths []string) bool {
	seen := map[string]bool{}
	for _, candidate := range paths {
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
