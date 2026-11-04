// check 与 reload 路径的完整校验：主配置、include 展开与逐个 server 块。
// 校验与解析共用同一份 schema（jsonConfig/jsonServer），
// 避免两套代码对配置结构的理解各自漂移。
package config

import (
	"errors"
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
func ValidateConfigFile(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config file failed: %w", err)
	}

	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return fmt.Errorf("parse config json failed: %w", err)
	}

	baseDir := filepath.Dir(configPath)

	if root.WAF != nil {
		switch root.WAF.Mode {
		case "", "enforce", "monitor", "disabled":
		default:
			return fmt.Errorf("waf.mode is invalid: %s (want enforce, monitor or disabled)", root.WAF.Mode)
		}
	}

	if err := root.Acme.validate(); err != nil {
		return err
	}
	if err := root.Security.validate(); err != nil {
		return err
	}
	// http-01 需要对外暴露 80 端口，但也可能由前置反代转发，因此只告警不阻断。
	root.Acme.warnHTTP01WithoutPort80(root.HTTP.Servers)

	serverCount := len(root.HTTP.Servers)
	for i := range root.HTTP.Servers {
		if err := validateServer(&root.HTTP.Servers[i], baseDir, baseDir,
			fmt.Sprintf("http.server[%d]", i), true); err != nil {
			return err
		}
	}

	includeServers, err := validateIncludes(root.HTTP.Include, baseDir)
	if err != nil {
		return err
	}
	// 站点允许只写在 include 目录里，主配置不出现 http.server；
	// 但整体一个站点都没有就等于没配服务。
	if serverCount+includeServers == 0 {
		return errors.New("no server configured: add http.server or a server file under http.include")
	}
	return nil
}

// validateIncludes 校验 include 目录/文件，返回其中有效的 server 块数量。
func validateIncludes(includes []string, rootBaseDir string) (int, error) {
	files, err := includeFiles(includes)
	if err != nil {
		return 0, err
	}
	for _, path := range files {
		if err := validateIncludeServerFile(path, rootBaseDir); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}

// includeFiles 把 http.include 展开成具体的 .json 文件列表：目录递归收集，
// 单个文件必须带 .json 后缀。路径统一按 FAST_HTTPS_BASE_DIR 解析，
// 校验与加载因此用同一个基准，不再依赖进程工作目录。
func includeFiles(includes []string) ([]string, error) {
	var files []string
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			return nil, errors.New("http.include must contain non-empty paths")
		}

		fullPath := absConfigPath(includePath)
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
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func validateIncludeServerFile(includeFilePath, rootBaseDir string) error {
	content, err := os.ReadFile(includeFilePath)
	if err != nil {
		return fmt.Errorf("read include file failed: %w", err)
	}

	var s jsonServer
	if err := unmarshalConfig(content, &s); err != nil {
		return fmt.Errorf("parse include json failed (%s): %w", includeFilePath, err)
	}

	fileBaseDir := filepath.Dir(includeFilePath)
	return validateServer(&s, fileBaseDir, rootBaseDir, includeFilePath, true)
}

// validateServer 校验一个 server 块（主配置数组元素或 include 文件）。
func validateServer(s *jsonServer, certBaseDir, rootBaseDir, where string, requireCerts bool) error {
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
		if requireCerts && !pathExists(s.SSLCertificate, certBaseDir, rootBaseDir) {
			return fmt.Errorf("%s: ssl_certificate not found: %s", where, s.SSLCertificate)
		}
		if requireCerts && !pathExists(s.SSLCertificateKey, certBaseDir, rootBaseDir) {
			return fmt.Errorf("%s: ssl_certificate_key not found: %s", where, s.SSLCertificateKey)
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
	if strings.TrimSpace(path) == "" {
		return false
	}
	// Certificate paths in the main config are relative to FAST_HTTPS_BASE_DIR
	// (the process working directory for non-package builds, /usr/share/fast-https
	// for packages), which is also how tls.LoadX509KeyPair resolves them after
	// ResolveRuntimePaths. Paths inside an included file may instead be relative
	// to that file or the main config.
	candidates := []string{path, absConfigPath(path)}
	if !filepath.IsAbs(path) {
		candidates = append(candidates,
			filepath.Join(baseDir, path),
			filepath.Join(rootBaseDir, path),
		)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}
