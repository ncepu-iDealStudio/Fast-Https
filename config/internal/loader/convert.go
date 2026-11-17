// Package loader 把 schema 转成运行期配置并补齐默认值，同时提供配置的
// 加载/重载入口。校验在 validate 包，本包只在重载入口里调用它。
package loader

import (
	"fmt"
	"net/netip"
	"os"
	"strings"

	"fast-https/config/internal/acme"
	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/source"
	"fast-https/config/internal/validate"
	"fast-https/infra/logging"
)

// Init 加载编译期基址上的主配置文件。
func Init() error {
	return LoadFile(model.CONFIG_FILE_PATH)
}

// LoadFile 读取一份配置与 MIME 表，然后整体替换配置快照。
// 任何一步失败都不改动现有快照，读取方不会看到清空或半成品。
func LoadFile(path string) error {
	cfg, err := processRoot(path)
	if err != nil {
		return err
	}
	if err := checkLoadTimeSymlinks(&cfg); err != nil {
		return err
	}
	mime, err := source.LoadContentTypes(path)
	if err != nil {
		return err
	}
	model.Store(&cfg, mime)
	return nil
}

// checkLoadTimeSymlinks 在加载层兜底执行 check-at-load 策略。
// 默认策略不能只靠 validate 生效：直接调用 LoadFile/Init 的入口
// （InitSystem、acme/管理命令）同样要拒绝带符号链接的静态根。
// 根不存在或不可读时跳过，交给运行期报错。
func checkLoadTimeSymlinks(cfg *model.Fast_Https) error {
	for i := range cfg.Servers {
		server := &cfg.Servers[i]
		for j := range server.Path {
			path := &server.Path[j]
			if path.PathType != model.LOCAL || path.SymlinkPolicy != model.SYMLINK_CHECK_AT_LOAD {
				continue
			}
			link, err := paths.FirstSymlinkUnder(path.Root)
			if err != nil || link == "" {
				continue
			}
			return fmt.Errorf("server %q location %q: symlink found under root: %s (symlink_policy=check-at-load; remove the link or set symlink_policy to allow/reject)",
				server.ServerName, path.PathName, link)
		}
	}
	return nil
}

// Reload 重新加载编译期基址上的主配置。
func Reload() error {
	return ReloadFile(model.CONFIG_FILE_PATH)
}

// ReloadFile 先校验再加载，校验或加载失败都保留原快照。
func ReloadFile(path string) error {
	if err := validate.ValidateConfigFile(path); err != nil {
		return err
	}
	return LoadFile(path)
}

func processRoot(configPath string) (model.Fast_Https, error) {
	var cfg model.Fast_Https
	content, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("error reading config file: %w", err)
	}

	var jc schema.Config
	if err := schema.Unmarshal(content, &jc); err != nil {
		return cfg, fmt.Errorf("error unmarshaling config: %w", err)
	}

	cfg.Pid = jc.Pid
	cfg.LogRoot = jc.LogRoot
	cfg.ServerEngine = model.Engine{
		IsMaster:     jc.Engine.IsMaster,
		Id:           jc.Engine.ID,
		RegisterPort: jc.Engine.RegisterPort,
		SlaveIp:      jc.Engine.SlaveIP,
		SlavePort:    jc.Engine.SlavePort,
	}
	cfg.Waf = jc.WAF
	cfg.Acme = jc.Acme
	cfg.Security = jc.Security
	cfg.TLS = model.TLSConfig{DefaultCertificate: model.Certificate{
		Certificate: jc.TLS.DefaultCertificate.Certificate,
		Key:         jc.TLS.DefaultCertificate.Key,
	}}
	proxy, err := convertProxy(jc.Proxy)
	if err != nil {
		return cfg, err
	}
	cfg.Proxy = proxy
	if err := processHttp(&cfg, jc.HTTP); err != nil {
		return cfg, err
	}
	SetDefault(&cfg)
	return cfg, nil
}

// convertProxy 把 proxy 段转成运行期策略：preserve_host 缺省为 true，
// trusted_proxies 在这里编译成网段，运行期不再解析字符串。
func convertProxy(jp schema.Proxy) (model.Proxy, error) {
	proxy := model.Proxy{
		ForwardedForMaxHops: jp.ForwardedFor.MaxHops,
		PreserveHost:        jp.PreserveHost == nil || *jp.PreserveHost,
		UpstreamTimeout:     jp.UpstreamTimeout,
		TunnelIdleTimeout:   jp.TunnelIdleTimeout,
	}
	for i, raw := range jp.TrustedProxies {
		item := strings.TrimSpace(raw)
		prefix, err := parseTrustedProxy(item)
		if err != nil {
			return proxy, fmt.Errorf("proxy.trusted_proxies[%d]: %w", i, err)
		}
		proxy.TrustedProxies = append(proxy.TrustedProxies, item)
		proxy.TrustedPrefixes = append(proxy.TrustedPrefixes, prefix)
	}
	return proxy, nil
}

// parseTrustedProxy 接受单地址或 CIDR：单地址补全 /32 或 /128。
func parseTrustedProxy(item string) (netip.Prefix, error) {
	if item == "" {
		return netip.Prefix{}, fmt.Errorf("must not be empty")
	}
	if prefix, err := netip.ParsePrefix(item); err == nil {
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(item)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid IP or CIDR: %q", item)
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

func processHttp(cfg *model.Fast_Https, http schema.HTTP) error {
	cfg.Include = http.Include
	cfg.DefaultType = http.DefaultType
	cfg.Limit = http.Limit
	cfg.BlackList = http.BlackList
	cfg.LogFormat = http.LogFormat
	for _, js := range http.Servers {
		cfg.Servers = append(cfg.Servers, convertServers(js)...)
	}
	return processIncludeCfg(cfg)
}

// processIncludeCfg 展开 include 指向的站点配置。include 文件是一个
// server 块，与 http.server 数组元素同构。任何 include 文件读不了、
// 解不开都返回错误：reload 路径上不能因为 Fatal 把整个进程带走。
func processIncludeCfg(cfg *model.Fast_Https) error {
	// 先把 include 路径按 ETC 基址展开并写回快照：否则相对路径
	// 会跟着启动目录走，装好包后在别的目录执行就找不到 conf.d。
	for i := range cfg.Include {
		cfg.Include[i] = paths.ETCPath(cfg.Include[i])
	}

	files, err := source.IncludeFiles(cfg.Include, model.FAST_HTTPS_ETC_DIR, "", ".")
	if err != nil {
		return fmt.Errorf("process include config failed: %w", err)
	}
	for _, path := range files {
		logging.ConsoleLog.Info().Msgf("Include config dir: %s", path)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading include config file %s: %w", path, err)
		}
		var js schema.Server
		if err := schema.Unmarshal(content, &js); err != nil {
			return fmt.Errorf("error unmarshaling include config %s: %w", path, err)
		}

		cfg.Servers = append(cfg.Servers, convertServers(js)...)
	}

	return nil
}

// convertServers 把一个 JSON server 块展开成运行期 server 块：写几个 listen
// 就得到几个，server_name、location 与证书共用。多端口因此只是配置写法，
// 监听与路由仍按"一块一个端口"处理。
func convertServers(js schema.Server) []model.Server {
	base := model.Server{
		ServerName:        js.ServerName,
		SSLCertificate:    js.SSLCertificate,
		SSLCertificateKey: js.SSLCertificateKey,
	}
	for _, jl := range js.Locations {
		base.Path = append(base.Path, convertPath(jl))
	}

	servers := make([]model.Server, 0, len(js.Listen))
	for _, listen := range js.Listen {
		server := base
		// Path 是切片：复制一份，避免多个监听共享同一个底层数组，
		// 后续按 server 改写路径（ResolveRuntimePaths）时互相影响。
		server.Path = append([]model.Path(nil), base.Path...)
		server.Listen = listen
		servers = append(servers, server)
	}
	return servers
}

func convertPath(jl schema.Path) model.Path {
	return model.Path{
		PathName:       jl.URL,
		PathType:       parsePathType(jl.Type, jl.ProxyPass),
		Root:           jl.Root,
		SymlinkPolicy:  model.NormalizeSymlinkPolicy(jl.SymlinkPolicy),
		Index:          jl.Index,
		Rewrite:        jl.Rewrite,
		Trys:           jl.Try,
		ProxyData:      trimProxyPass(jl.ProxyPass),
		ProxySetHeader: jl.ProxySetHeader,
		AppFireWall:    jl.AppFireWall,
		ProxyCache:     jl.ProxyCache,
		Limit:          jl.Limit,
		Auth:           jl.Auth,
		ErrorNotFound:  jl.ErrorPage.NotFound,
		ErrorProxy:     jl.ErrorPage.Proxy,
		Zip:            parseZip(jl.Zip),
		ZipMinLength:   jl.ZipMinLength,
		ZipSkipTypes:   normalizeMIMETypes(jl.ZipSkipTypes),
		AllowMethods:   normalizeMethods(jl.AllowMethods),
	}
}

// normalizeMIMETypes 统一小写、去空白并按首次出现去重；
// 空列表返回 nil，让静态层回落到内置的已压缩类型清单。
func normalizeMIMETypes(types []string) []string {
	if len(types) == 0 {
		return nil
	}
	out := make([]string, 0, len(types))
	seen := make(map[string]bool, len(types))
	for _, item := range types {
		v := strings.ToLower(strings.TrimSpace(item))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// normalizeMethods 统一成大写并去掉空白，让配置写成 get 也能工作；
// HTTP 方法本身大小写敏感，但配置里的大小写手滑不该变成 405。
func normalizeMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		trimmed := strings.ToUpper(strings.TrimSpace(method))
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseZip(zip []string) uint16 {
	var zipType uint16 = model.ZIP_NONE
	if len(zip) == 1 {
		if zip[0] == "br" {
			zipType = model.ZIP_BR
		}
		if zip[0] == "gzip" {
			zipType = model.ZIP_GZIP
		}
	} else if len(zip) == 2 {
		if zip[0] == "gzip" && zip[1] == "br" {
			zipType = model.ZIP_GZIP_BR
		}
		if zip[1] == "gzip" && zip[0] == "br" {
			zipType = model.ZIP_GZIP_BR
		}
	}
	return zipType
}

func parsePathType(typeStr, proxyPass string) uint16 {
	switch typeStr {
	case "local":
		return model.LOCAL
	case "rewrite":
		return model.REWRITE
	case "devmod":
		return model.DEVMOD
	case "proxy":
		// proxy_pass 缺少 "scheme:" 前缀时按 local 兜底，
		// 不再像旧实现那样对负数下标切片 panic
		if scheme, _, ok := strings.Cut(proxyPass, ":"); ok {
			if scheme == "http" {
				return model.PROXY_HTTP
			}
			if scheme == "https" {
				return model.PROXY_HTTPS
			}
		}
	}
	return model.LOCAL
}

func trimProxyPass(proxyData string) string {
	return strings.TrimPrefix(strings.TrimPrefix(proxyData, "https://"), "http://")
}

// SetDefault 补齐系统级默认值，并把相对路径解析成绝对路径。
func SetDefault(cfg *model.Fast_Https) {
	if cfg.Limit.MaxHeaderSize == 0 {
		cfg.Limit.MaxHeaderSize = model.DEFAULT_MAX_HEADER_SIZE
	}
	if cfg.Limit.MaxBodySize == 0 {
		cfg.Limit.MaxBodySize = model.DEFAULT_MAX_BODY_SIZE
	}
	if cfg.Limit.ReadTimeout == 0 {
		cfg.Limit.ReadTimeout = model.DEFAULT_READ_TIMEOUT
	}
	if cfg.Limit.WriteTimeout == 0 {
		cfg.Limit.WriteTimeout = model.DEFAULT_WRITE_TIMEOUT
	}
	if cfg.Limit.BodyTimeout == 0 {
		cfg.Limit.BodyTimeout = model.DEFAULT_BODY_TIMEOUT
	}
	if cfg.DefaultType == "" {
		cfg.DefaultType = model.HTTP_DEFAULT_CONTENT_TYPE
	}

	if cfg.LogRoot == "" {
		cfg.LogRoot = model.DEFAULT_LOG_ROOT
	}
	// acme 段的默认值只在解析完成后补一次：校验路径上不能补默认值，
	// 否则 check 与 load 对同一份文件的判断会不一致。
	acme.ApplyDefaults(cfg.Acme)
	model.ApplySecurityDefaults(cfg.Security)
	paths.ResolveRuntimePaths(cfg)
}
