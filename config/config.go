// Package config 负责加载、校验并热重载 JSON 配置（含 conf.d 包含目录）。
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fast-https/infra/jsonc"
	"fast-https/infra/logging"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	ZIP_NONE    = 0
	ZIP_GZIP    = 1
	ZIP_BR      = 2
	ZIP_GZIP_BR = 10

	Host        = 100
	XRealIp     = 101
	XForwardFor = 102
)

/*
// path type
*/
const (
	LOCAL       = 0
	PROXY_HTTP  = 1
	PROXY_HTTPS = 2
	PROXY_TCP   = 3
	REWRITE     = 4

	DEVMOD = 10
)

// ---------- 配置类型 ----------
// json 标签即配置文件键名；历史拼写（如 http.blaklist）原样保留以兼容存量配置。

type ErrorPath struct {
	Path404 string `json:"path404"`
	Path500 string `json:"path500"`
}

type Header struct {
	HeaderKey   string `json:"HeaderKey"`
	HeaderValue string `json:"HeaderValue"`
}

type Cache struct {
	Path    string   `json:"path"`
	Valid   []string `json:"valid"`
	Key     string   `json:"key"`
	Methods []string `json:"methods"` // 允许读写的请求方法，留空等价于 GET 与 HEAD
	MaxSize int      `json:"max_size"`
}

type PathAuth struct {
	AuthType string `json:"type"`
	User     string `json:"user"`
	Pswd     string `json:"pswd"`
}

type PathLimit struct {
	Size    int  `json:"mem"`
	Rate    int  `json:"rate"`
	Burst   int  `json:"burst"`
	Nodelay bool `json:"nodelay"`
}

type ServerLimit struct {
	MaxBodySize   int `json:"max_body_size"`
	MaxHeaderSize int `json:"max_header_size"`
	Rate          int `json:"limit"`
	Burst         int `json:"burst"`
}

type Try struct {
	Uri   string   `json:"uri"`
	Files []string `json:"file"`
	Next  string   `json:"next"`
}

type Path struct {
	PathName       string
	PathType       uint16
	Zip            uint16
	Root           string
	Index          []string
	Rewrite        string
	Trys           []Try
	ProxyData      string
	ProxySetHeader []Header
	AppFireWall    []string
	ProxyCache     Cache
	Limit          PathLimit
	Auth           PathAuth
	ErrorNotFound  string
	ErrorProxy     string
	AllowMethods   []string
}

type Server struct {
	Listen string

	ServerName        string
	SSLCertificate    string
	SSLCertificateKey string
	Path              []Path
}

type Engine struct {
	IsMaster     bool
	Id           int
	RegisterPort int    // master uses, default 9099
	SlaveIp      string // slave uses
	SlavePort    int    // slave uses
}

// WAFConfig 是全局应用防火墙（GuardianWAF）的最小配置。
// 不配置 waf 块且无任何 location 配置 appfirewall 时不构建引擎（零开销）。
type WAFConfig struct {
	// Mode 引擎模式：enforce（默认，命中阻断）、monitor（只记录不阻断）、
	// disabled（等价于不启用）。
	Mode string `json:"mode"`
}

type Fast_Https struct {
	ErrorPage ErrorPath
	Error_log string
	Pid       string
	LogRoot   string

	Servers                   []Server
	ServerEngine              Engine
	Limit                     ServerLimit
	BlackList                 []string
	LogFormat                 []string
	Include                   []string
	DefaultType               string
	Waf                       *WAFConfig
	Acme                      *AcmeConfig
	Security                  *SecurityConfig
	ServerNamesHashBucketSize uint16
	ClientHeaderBufferSize    uint16
	LargeClientHeaderBuffers  uint8
	ClientMaxBodySize         uint8
	KeepaliveTimeout          uint8
	AutoIndex                 string
	AutoIndexExactSize        string
	AutoIndexLocaltime        string
	Sendfile                  string
	TcpNopush                 string
	TcpNodelay                string
}

// ---------- 配置文件 JSON schema ----------
// 配置文件是纯 JSON，直接用标准库解析成 schema，再转换为运行时配置。
// 单独的 schema 结构体让文件结构与运行时结构各自演进，互不牵连。

// unmarshalConfig 解析配置文件。格式是 infra/jsonc 支持的 JSONC：允许
// // 与 /* */ 注释、尾随逗号，解析结果与纯 JSON 完全一致。
func unmarshalConfig(data []byte, v any) error {
	return jsonc.Unmarshal(data, v)
}

// flexString 兼容 "listen": 8080（数字）与 "listen": "8080"（字符串）两种写法。
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*s = flexString(strconv.FormatFloat(f, 'f', -1, 64))
	return nil
}

// flexList 兼容 "listen": 8080、"listen": "8080" 与
// "listen": ["80", "443 ssl h2"]：一个 server 块可以同时监听多个端口。
// 加载阶段会把多监听展开成多个运行期 server 块（同 server_name、同 location、
// 同证书），端口监听与路由表因此不需要知道"一个块多个监听"这件事。
type flexList []string

func (l *flexList) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		list := make(flexList, 0, len(items))
		for _, item := range items {
			var one flexString
			if err := one.UnmarshalJSON(item); err != nil {
				return err
			}
			list = append(list, string(one))
		}
		*l = list
		return nil
	}
	var one flexString
	if err := one.UnmarshalJSON(trimmed); err != nil {
		return err
	}
	*l = flexList{string(one)}
	return nil
}

// MarshalJSON 单个监听按标量输出（纯端口写数字），既有配置与 site show 的
// 写法保持不变；多个监听才写成数组。
func (l flexList) MarshalJSON() ([]byte, error) {
	if len(l) == 1 {
		return json.Marshal(listenValue(l[0]))
	}
	values := make([]any, 0, len(l))
	for _, item := range l {
		values = append(values, listenValue(item))
	}
	return json.Marshal(values)
}

// listenValue 纯数字监听写成 JSON 数字，带 "ssl"/"h2" 时保持字符串。
func listenValue(listen string) any {
	if port, err := strconv.Atoi(strings.TrimSpace(listen)); err == nil {
		return port
	}
	return listen
}

type jsonPath struct {
	URL            string        `json:"url"`
	Type           string        `json:"type"`
	Root           string        `json:"root"`
	Index          []string      `json:"index"`
	Rewrite        string        `json:"rewrite"`
	ProxyPass      string        `json:"proxy_pass"`
	ProxySetHeader []Header      `json:"proxy_set_header"`
	AppFireWall    []string      `json:"appfirewall"`
	ProxyCache     Cache         `json:"proxy_cache"`
	Limit          PathLimit     `json:"limit"`
	Auth           PathAuth      `json:"auth"`
	ErrorPage      jsonErrorPage `json:"error_page"`
	Zip            []string      `json:"zip"`
	Try            []Try         `json:"try"`
	AllowMethods   []string      `json:"allow_methods"`
}

type jsonErrorPage struct {
	NotFound string `json:"not_found"`
	Proxy    string `json:"proxy"`
}

type jsonServer struct {
	Listen            flexList   `json:"listen"`
	ServerName        string     `json:"server_name"`
	SSLCertificate    string     `json:"ssl_certificate"`
	SSLCertificateKey string     `json:"ssl_certificate_key"`
	Locations         []jsonPath `json:"location"`
}

type jsonEngine struct {
	IsMaster     bool   `json:"is_master"`
	ID           int    `json:"id"`
	RegisterPort int    `json:"register_port"`
	SlaveIP      string `json:"slave_ip"`
	SlavePort    int    `json:"slave_port"`
}

type jsonHTTP struct {
	Include     []string     `json:"include"`
	DefaultType string       `json:"default_type"`
	Limit       ServerLimit  `json:"servers_limit"`
	BlackList   []string     `json:"blaklist"` // 历史拼写，配置兼容
	LogFormat   []string     `json:"log_format"`
	Servers     []jsonServer `json:"server"`
}

type jsonConfig struct {
	Pid      string          `json:"pid"`
	LogRoot  string          `json:"log_root"`
	Engine   jsonEngine      `json:"engine"`
	WAF      *WAFConfig      `json:"waf"`
	Acme     *AcmeConfig     `json:"acme"`
	Security *SecurityConfig `json:"security"`
	HTTP     jsonHTTP        `json:"http"`
}

// 运行时配置快照。reload 校验通过后整体替换快照，读取端要么看到完整的
// 旧配置、要么看到完整的新配置，失败时旧快照保持不动。
// （架构收敛计划阶段 4b：消除对进程级全局配置的无同步读写。）
var (
	currentConfig atomic.Pointer[Fast_Https]
	currentMIME   atomic.Pointer[map[string]string]
)

func init() {
	currentConfig.Store(&Fast_Https{})
	emptyMIME := map[string]string{}
	currentMIME.Store(&emptyMIME)
}

// Current 返回当前生效的配置快照。尚未加载配置时是零值快照。
func Current() *Fast_Https {
	return currentConfig.Load()
}

// MIME 返回当前生效的 MIME 表。表在发布后不再修改，读取方无需加锁。
func MIME() map[string]string {
	return *currentMIME.Load()
}

// Store 替换配置快照。LoadFile/ReloadFile 在校验全部通过后调用；
// 导出是为了测试能够保存与恢复快照。
func Store(cfg *Fast_Https, mime map[string]string) {
	if cfg != nil {
		currentConfig.Store(cfg)
	}
	if mime != nil {
		currentMIME.Store(&mime)
	}
}

// Init the whole config module
func Init() error {
	return LoadFile(CONFIG_FILE_PATH)
}

// LoadFile reads one config file and the MIME table, then replaces the
// config snapshot with both. Neither changes on error, and readers never
// observe a cleared or half-filled value.
func LoadFile(path string) error {
	cfg, err := processRoot(path)
	if err != nil {
		return err
	}
	mime, err := loadContentTypes()
	if err != nil {
		return err
	}
	Store(&cfg, mime)
	return nil
}

// Reload reloads the runtime config file.
// An invalid file leaves the current snapshot unchanged.
func Reload() error {
	return ReloadFile(CONFIG_FILE_PATH)
}

// ReloadFile validates path before replacing the config snapshot.
// Validation or load errors leave the previous config in place.
func ReloadFile(path string) error {
	if err := ValidateConfigFile(path); err != nil {
		return err
	}
	return LoadFile(path)
}

// CheckConfig check whether config is correct
func CheckConfig() error {
	return ValidateConfigFile(CONFIG_FILE_PATH)
}

// ValidateConfigFile validates the main json config file and included json files.
// 校验与解析共用同一份 schema（jsonConfig/jsonServer），
// 避免两套代码对配置结构的理解各自漂移。
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
	if len(root.HTTP.Servers) == 0 {
		return errors.New("http.server must contain at least one server")
	}

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

	for i := range root.HTTP.Servers {
		if err := validateServer(&root.HTTP.Servers[i], baseDir, baseDir,
			fmt.Sprintf("http.server[%d]", i), true); err != nil {
			return err
		}
	}

	return validateIncludes(root.HTTP.Include, baseDir)
}

func validateIncludes(includes []string, rootBaseDir string) error {
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			return errors.New("http.include must contain non-empty paths")
		}

		fullPath := includePath
		if !filepath.IsAbs(includePath) {
			if _, err := os.Stat(includePath); err != nil {
				// 与 processIncludeCfg 用同一个基准，包安装后 include 才指向
				// /usr/share/fast-https/config/conf.d 而不是配置目录下的 config/。
				fullPath = absConfigPath(includePath)
			}
		}

		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("include path not found: %s", fullPath)
		}

		if info.IsDir() {
			err = filepath.Walk(fullPath, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if info.IsDir() || filepath.Ext(path) != ".json" {
					return nil
				}
				return validateIncludeServerFile(path, rootBaseDir)
			})
			if err != nil {
				return err
			}
			continue
		}

		if filepath.Ext(fullPath) != ".json" {
			return fmt.Errorf("include file must be .json: %s", fullPath)
		}

		if err := validateIncludeServerFile(fullPath, rootBaseDir); err != nil {
			return err
		}
	}

	return nil
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

// content types of server
// mime 文件缺失或非法返回错误：启动时由组合根决定是否退出，
// reload 时保留旧配置继续服务。
func loadContentTypes() (map[string]string, error) {
	confBytes, err := os.ReadFile(MIME_FILE_PATH)
	if err != nil {
		return nil, fmt.Errorf("can't open mime.types file: %w", err)
	}

	mime := make(map[string]string)
	err = json.Unmarshal(confBytes, &mime)
	if err != nil {
		return nil, fmt.Errorf("can't unmarshal mime.json file: %w", err)
	}

	return mime, nil
}

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
	for i := range cfg.Include {
		// 先按 FAST_HTTPS_BASE_DIR 展开，再遍历。否则相对路径会跟着
		// 启动目录走：装好包后在别的目录执行就找不到 conf.d。
		item := absConfigPath(cfg.Include[i])
		cfg.Include[i] = item

		err := filepath.Walk(item, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// 跳过目录 "." 和 ".."
			if info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}

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
			return nil
		})
		if err != nil {
			return fmt.Errorf("process include config failed: %w", err)
		}
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

// ResolveRuntimePaths turns site, certificate, log and include paths into
// absolute paths before the process listens. Later directory changes then
// still open the files that were configured.
func ResolveRuntimePaths(cfg *Fast_Https) {
	cfg.LogRoot = absConfigPath(cfg.LogRoot)
	for i := range cfg.Include {
		cfg.Include[i] = absConfigPath(cfg.Include[i])
	}
	for i := range cfg.Servers {
		server := &cfg.Servers[i]
		server.SSLCertificate = absConfigPath(server.SSLCertificate)
		server.SSLCertificateKey = absConfigPath(server.SSLCertificateKey)
		for j := range server.Path {
			server.Path[j].Root = absConfigPath(server.Path[j].Root)
			server.Path[j].ProxyCache.Path = absConfigPath(server.Path[j].ProxyCache.Path)
			server.Path[j].ErrorNotFound = absConfigPath(server.Path[j].ErrorNotFound)
			server.Path[j].ErrorProxy = absConfigPath(server.Path[j].ErrorProxy)
		}
	}
}

// absConfigPath 把配置里的相对路径按 FAST_HTTPS_BASE_DIR 解析成绝对路径，
// 而不是按进程的工作目录。装了包之后从任意目录执行 fast-https 都指向同一份资源。
// 非包构建的 FAST_HTTPS_BASE_DIR 是 "./"，解析结果仍等于启动目录下的路径。
func absConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	base, err := filepath.Abs(FAST_HTTPS_BASE_DIR)
	if err != nil {
		return path
	}
	return filepath.Join(base, path)
}
