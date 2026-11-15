// schema → 运行期结构的转换：processRoot 是入口，convertServers 把一个
// JSON server 块按监听展开成多个运行期 server，SetDefault 补齐默认值。
package config

import (
	"fmt"
	"os"
	"strings"

	"fast-https/infra/logging"
)

func processRoot(configPath string) (Fast_Https, error) {
	var cfg Fast_Https
	content, err := os.ReadFile(configPath)
	if err != nil {
		return cfg, fmt.Errorf("error reading config file: %w", err)
	}

	var jc jsonConfig
	if err := unmarshalConfig(content, &jc); err != nil {
		return cfg, fmt.Errorf("error unmarshaling config: %w", err)
	}

	cfg.Pid = jc.Pid
	cfg.LogRoot = jc.LogRoot
	cfg.ServerEngine = Engine{
		IsMaster:     jc.Engine.IsMaster,
		Id:           jc.Engine.ID,
		RegisterPort: jc.Engine.RegisterPort,
		SlaveIp:      jc.Engine.SlaveIP,
		SlavePort:    jc.Engine.SlavePort,
	}
	cfg.Waf = jc.WAF
	cfg.Acme = jc.Acme
	cfg.Security = jc.Security
	if err := processHttp(&cfg, jc.HTTP); err != nil {
		return cfg, err
	}
	SetDefault(&cfg)
	return cfg, nil
}

func processHttp(cfg *Fast_Https, http jsonHTTP) error {
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
func processIncludeCfg(cfg *Fast_Https) error {
	// 先把 include 路径按 ETC 基址展开并写回快照：否则相对路径
	// 会跟着启动目录走，装好包后在别的目录执行就找不到 conf.d。
	for i := range cfg.Include {
		cfg.Include[i] = absETCPath(cfg.Include[i])
	}

	files, err := includeFiles(cfg.Include, FAST_HTTPS_ETC_DIR, "", ".")
	if err != nil {
		return fmt.Errorf("process include config failed: %w", err)
	}
	for _, path := range files {
		logging.ConsoleLog.Info().Msgf("Include config dir: %s", path)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading include config file %s: %w", path, err)
		}
		var js jsonServer
		if err := unmarshalConfig(content, &js); err != nil {
			return fmt.Errorf("error unmarshaling include config %s: %w", path, err)
		}

		cfg.Servers = append(cfg.Servers, convertServers(js)...)
	}

	return nil
}

// convertServers 把一个 JSON server 块展开成运行期 server 块：写几个 listen
// 就得到几个，server_name、location 与证书共用。多端口因此只是配置写法，
// 监听与路由仍按"一块一个端口"处理。
func convertServers(js jsonServer) []Server {
	base := Server{
		ServerName:        js.ServerName,
		SSLCertificate:    js.SSLCertificate,
		SSLCertificateKey: js.SSLCertificateKey,
	}
	for _, jl := range js.Locations {
		base.Path = append(base.Path, convertPath(jl))
	}

	servers := make([]Server, 0, len(js.Listen))
	for _, listen := range js.Listen {
		server := base
		// Path 是切片：复制一份，避免多个监听共享同一个底层数组，
		// 后续按 server 改写路径（ResolveRuntimePaths）时互相影响。
		server.Path = append([]Path(nil), base.Path...)
		server.Listen = listen
		servers = append(servers, server)
	}
	return servers
}

func convertPath(jl jsonPath) Path {
	return Path{
		PathName:       jl.URL,
		PathType:       parsePathType(jl.Type, jl.ProxyPass),
		Root:           jl.Root,
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
		AllowMethods:   normalizeMethods(jl.AllowMethods),
	}
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
	var zipType uint16 = ZIP_NONE
	if len(zip) == 1 {
		if zip[0] == "br" {
			zipType = ZIP_BR
		}
		if zip[0] == "gzip" {
			zipType = ZIP_GZIP
		}
	} else if len(zip) == 2 {
		if zip[0] == "gzip" && zip[1] == "br" {
			zipType = ZIP_GZIP_BR
		}
		if zip[1] == "gzip" && zip[0] == "br" {
			zipType = ZIP_GZIP_BR
		}
	}
	return zipType
}

func parsePathType(typeStr, proxyPass string) uint16 {
	switch typeStr {
	case "local":
		return LOCAL
	case "rewrite":
		return REWRITE
	case "devmod":
		return DEVMOD
	case "proxy":
		// proxy_pass 缺少 "scheme:" 前缀时按 local 兜底，
		// 不再像旧实现那样对负数下标切片 panic
		if scheme, _, ok := strings.Cut(proxyPass, ":"); ok {
			if scheme == "http" {
				return PROXY_HTTP
			}
			if scheme == "https" {
				return PROXY_HTTPS
			}
		}
	}
	return LOCAL
}

func trimProxyPass(proxyData string) string {
	return strings.TrimPrefix(strings.TrimPrefix(proxyData, "https://"), "http://")
}

func SetDefault(cfg *Fast_Https) {
	if cfg.Limit.MaxHeaderSize == 0 {
		cfg.Limit.MaxHeaderSize = DEFAULT_MAX_HEADER_SIZE
	}
	if cfg.Limit.MaxBodySize == 0 {
		cfg.Limit.MaxBodySize = DEFAULT_MAX_BODY_SIZE
	}
	if cfg.DefaultType == "" {
		cfg.DefaultType = HTTP_DEFAULT_CONTENT_TYPE
	}

	if cfg.LogRoot == "" {
		cfg.LogRoot = DEFAULT_LOG_ROOT
	}
	// acme 段的默认值只在解析完成后补一次：校验路径上不能补默认值，
	// 否则 check 与 load 对同一份文件的判断会不一致。
	cfg.Acme.applyDefaults()
	cfg.Security.applyDefaults()
	ResolveRuntimePaths(cfg)
}
