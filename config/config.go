// Package config 负责加载、校验并热重载 JSON 配置（含 conf.d 包含目录）。
package config

import (
	"encoding/json"
	"errors"
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
}

type jsonErrorPage struct {
	NotFound string `json:"not_found"`
	Proxy    string `json:"proxy"`
}

type jsonServer struct {
	Listen            flexString `json:"listen"`
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
	Pid     string     `json:"pid"`
	LogRoot string     `json:"log_root"`
	Engine  jsonEngine `json:"engine"`
	HTTP    jsonHTTP   `json:"http"`
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
func ValidateConfigFile(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config file failed: %w", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(content, &root); err != nil {
		return fmt.Errorf("parse config json failed: %w", err)
	}

	baseDir := filepath.Dir(configPath)
	httpMap, ok := asMap(root["http"])
	if !ok {
		return errors.New("missing http section")
	}

	servers, ok := asSlice(httpMap["server"])
	if !ok || len(servers) == 0 {
		return errors.New("http.server must contain at least one server")
	}

	for i, rawServer := range servers {
		serverMap, ok := asMap(rawServer)
		if !ok {
			return fmt.Errorf("http.server[%d] must be object", i)
		}
		if err := validateServerBlock(serverMap, baseDir, baseDir, fmt.Sprintf("http.server[%d]", i)); err != nil {
			return err
		}
	}

	if err := validateIncludes(httpMap, baseDir); err != nil {
		return err
	}

	return nil
}

func validateIncludes(httpMap map[string]interface{}, rootBaseDir string) error {
	rawInclude, exists := httpMap["include"]
	if !exists {
		return nil
	}

	includes, ok := asSlice(rawInclude)
	if !ok {
		return errors.New("http.include must be array")
	}

	for i, raw := range includes {
		includePath, ok := raw.(string)
		if !ok || strings.TrimSpace(includePath) == "" {
			return fmt.Errorf("http.include[%d] must be non-empty string", i)
		}

		fullPath := includePath
		if !filepath.IsAbs(includePath) {
			if _, err := os.Stat(includePath); err != nil {
				fullPath = filepath.Join(rootBaseDir, includePath)
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
				if err := validateIncludeServerFile(path, rootBaseDir); err != nil {
					return err
				}
				return nil
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

	var include map[string]interface{}
	if err := json.Unmarshal(content, &include); err != nil {
		return fmt.Errorf("parse include json failed (%s): %w", includeFilePath, err)
	}

	fileBaseDir := filepath.Dir(includeFilePath)
	return validateServerBlock(include, fileBaseDir, rootBaseDir, includeFilePath)
}

func validateServerBlock(serverMap map[string]interface{}, certBaseDir, rootBaseDir, where string) error {
	listen, ok := serverMap["listen"]
	if !ok {
		return fmt.Errorf("%s: missing listen", where)
	}

	listenStr, err := stringifyListen(listen)
	if err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	if err := validateListenPort(listenStr); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}

	serverName, ok := serverMap["server_name"].(string)
	if !ok || strings.TrimSpace(serverName) == "" {
		return fmt.Errorf("%s: server_name is required", where)
	}

	if strings.Contains(listenStr, "ssl") {
		crt, _ := serverMap["ssl_certificate"].(string)
		key, _ := serverMap["ssl_certificate_key"].(string)
		if strings.TrimSpace(crt) == "" || strings.TrimSpace(key) == "" {
			return fmt.Errorf("%s: ssl listen requires ssl_certificate and ssl_certificate_key", where)
		}
		if !pathExists(crt, certBaseDir, rootBaseDir) {
			return fmt.Errorf("%s: ssl_certificate not found: %s", where, crt)
		}
		if !pathExists(key, certBaseDir, rootBaseDir) {
			return fmt.Errorf("%s: ssl_certificate_key not found: %s", where, key)
		}
	}

	rawLocations, ok := asSlice(serverMap["location"])
	if !ok || len(rawLocations) == 0 {
		return fmt.Errorf("%s: location must contain at least one rule", where)
	}

	for i, raw := range rawLocations {
		location, ok := asMap(raw)
		if !ok {
			return fmt.Errorf("%s: location[%d] must be object", where, i)
		}

		url, _ := location["url"].(string)
		if strings.TrimSpace(url) == "" {
			return fmt.Errorf("%s: location[%d].url is required", where, i)
		}

		typeStr, _ := location["type"].(string)
		if typeStr == "" {
			typeStr = "local"
		}

		switch typeStr {
		case "local":
			root, _ := location["root"].(string)
			if strings.TrimSpace(root) == "" {
				return fmt.Errorf("%s: location[%d].root is required for local type", where, i)
			}
		case "proxy":
			pass, _ := location["proxy_pass"].(string)
			if strings.TrimSpace(pass) == "" {
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
	}

	return nil
}

func stringifyListen(v interface{}) (string, error) {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return "", errors.New("listen must be non-empty")
		}
		return t, nil
	case float64:
		return strconv.Itoa(int(t)), nil
	default:
		return "", errors.New("listen must be string or number")
	}
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
	// Certificate paths in the main config are relative to the process working
	// directory, which is also how tls.LoadX509KeyPair reads them. Paths inside
	// an included file may instead be relative to that file or the main config.
	candidates := []string{path}
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

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asSlice(v interface{}) ([]interface{}, bool) {
	s, ok := v.([]interface{})
	return s, ok
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
	if err := json.Unmarshal(content, &jc); err != nil {
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
		cfg.Servers = append(cfg.Servers, convertServer(js))
	}
	return processIncludeCfg(cfg)
}

// processIncludeCfg 展开 include 指向的站点配置。include 文件是一个
// server 块，与 http.server 数组元素同构。任何 include 文件读不了、
// 解不开都返回错误：reload 路径上不能因为 Fatal 把整个进程带走。
func processIncludeCfg(cfg *Fast_Https) error {
	for _, item := range cfg.Include {

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
			if err := json.Unmarshal(content, &js); err != nil {
				return fmt.Errorf("error unmarshaling include config %s: %w", path, err)
			}

			cfg.Servers = append(cfg.Servers, convertServer(js))
			return nil
		})
		if err != nil {
			return fmt.Errorf("process include config failed: %w", err)
		}
	}

	return nil
}

func convertServer(js jsonServer) Server {
	server := Server{
		Listen:            string(js.Listen),
		ServerName:        js.ServerName,
		SSLCertificate:    js.SSLCertificate,
		SSLCertificateKey: js.SSLCertificateKey,
	}
	for _, jl := range js.Locations {
		server.Path = append(server.Path, convertPath(jl))
	}
	return server
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
	}
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
		}
	}
}

func absConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
