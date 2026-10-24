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

	"github.com/spf13/viper"
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

type ErrorPath struct {
	Path404 string
	Path500 string
}

type Header struct {
	HeaderKey   string
	HeaderValue string
}

type Cache struct {
	Path    string
	Valid   []string
	Key     string
	Methods []string // 允许读写的请求方法，留空等价于 GET 与 HEAD
	MaxSize int      // 1023MB
}

type PathAuth struct {
	AuthType string
	User     string
	Pswd     string
}

type PathLimit struct {
	Size    int
	Rate    int
	Burst   int
	Nodelay bool
}

type ServerLimit struct {
	MaxBodySize   int
	MaxHeaderSize int
	Rate          int
	Burst         int
}

type Try struct {
	Uri   string
	Files []string
	Next  string
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

var rootViper = viper.New()

func getHeaders(v *viper.Viper, path string) []Header {
	headerKeys := v.GetStringSlice(path)
	var headers []Header
	for headerKey := range headerKeys {
		header := Header{
			HeaderKey: v.GetString(fmt.Sprintf("%s.%d.HeaderKey",
				path, headerKey)),
			HeaderValue: v.GetString(fmt.Sprintf("%s.%d.HeaderValue",
				path, headerKey)),
		}
		headers = append(headers, header)
	}
	return headers
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
	rootViper.SetConfigFile(configPath)

	err := rootViper.ReadInConfig()
	if err != nil {
		return cfg, fmt.Errorf("error reading config file: %s", err)
	}

	var parsed Fast_Https
	err = rootViper.Unmarshal(&parsed)
	if err != nil {
		return cfg, fmt.Errorf("error unmarshaling config: %w", err)
	}

	cfg.Pid = rootViper.GetString("pid")
	cfg.LogRoot = rootViper.GetString("log_root")
	processEngine(&cfg)
	if err := processHttp(&cfg); err != nil {
		return cfg, err
	}
	SetDefault(&cfg)
	return cfg, nil
}

func processEngine(cfg *Fast_Https) {
	cfg.ServerEngine = Engine{
		IsMaster:     rootViper.GetBool("engine.is_master"),
		Id:           rootViper.GetInt("engine.id"),
		RegisterPort: rootViper.GetInt("engine.register_port"),
		SlaveIp:      rootViper.GetString("engine.slave_ip"),
		SlavePort:    rootViper.GetInt("engine.slave_port"),
	}
}

func processHttp(cfg *Fast_Https) error {
	cfg.Include = rootViper.GetStringSlice("http.include")
	cfg.DefaultType = rootViper.GetString("http.default_type")
	cfg.Limit = ServerLimit{
		MaxHeaderSize: rootViper.GetInt("http.servers_limit.max_header_size"),
		MaxBodySize:   rootViper.GetInt("http.servers_limit.max_body_size"),
		Rate:          rootViper.GetInt("http.servers_limit.limit"),
		Burst:         rootViper.GetInt("http.servers_limit.burst"),
	}

	cfg.BlackList = rootViper.GetStringSlice("http.blaklist")
	cfg.LogFormat = rootViper.GetStringSlice("http.log_format")
	cfg.Servers = processHttpServer("http.server")
	return processIncludeCfg(cfg)
}

// processIncludeCfg 展开	include 指向的站点配置。任何 include 文件读不了、
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
			incViper := viper.New()
			incViper.SetConfigFile(path)
			if err := incViper.ReadInConfig(); err != nil {
				return fmt.Errorf("error reading include config file %s: %w", path, err)
			}
			var config Fast_Https
			if err := incViper.Unmarshal(&config); err != nil {
				return fmt.Errorf("error unmarshaling include config %s: %w", path, err)
			}

			server := Server{
				Listen:            incViper.GetString("listen"),
				ServerName:        incViper.GetString("server_name"),
				SSLCertificate:    incViper.GetString("ssl_certificate"),
				SSLCertificateKey: incViper.GetString("ssl_certificate_key"),
			}

			locationKeys := incViper.GetStringSlice("location")

			var paths []Path
			for locationKey := range locationKeys {
				path := processHttpServerPath(incViper, "location", locationKey)
				paths = append(paths, path)
			}
			server.Path = paths

			cfg.Servers = append(cfg.Servers, server)
			return nil
		})
		if err != nil {
			return fmt.Errorf("process include config failed: %w", err)
		}
	}

	return nil
}

func processHttpServer(pathPrefix string) []Server {
	var servers []Server

	serverKeys := rootViper.GetStringSlice(pathPrefix)
	for serverKey := range serverKeys {

		server := Server{
			Listen: rootViper.GetString(fmt.Sprintf("%s.%d.listen",
				pathPrefix, serverKey)),
			ServerName: rootViper.GetString(fmt.Sprintf("%s.%d.server_name",
				pathPrefix, serverKey)),
			SSLCertificate: rootViper.GetString(fmt.Sprintf("%s.%d.ssl_certificate",
				pathPrefix, serverKey)),
			SSLCertificateKey: rootViper.GetString(fmt.Sprintf("%s.%d.ssl_certificate_key",
				pathPrefix, serverKey)),
		}

		locationPrefix := fmt.Sprintf("%s.%d.location", pathPrefix, serverKey)
		locationKeys := rootViper.GetStringSlice(locationPrefix)

		var paths []Path
		for locationKey := range locationKeys {
			path := processHttpServerPath(rootViper, locationPrefix, locationKey)
			paths = append(paths, path)
		}
		server.Path = paths
		servers = append(servers, server)
	}
	return servers
}

func processHttpServerPath(v *viper.Viper, pathPrefix string, locationKey int) Path {
	return Path{
		PathName: v.GetString(fmt.Sprintf("%s.%d.url", pathPrefix, locationKey)),
		//PathType:       v.GetUint16(fmt.Sprintf("%s.%d.path_type",
		// pathPrefix, locationKey)),
		//Zip:            v.GetUint16(fmt.Sprintf("%s.%d.zip",
		// pathPrefix, locationKey)),
		Root: v.GetString(fmt.Sprintf("%s.%d.root",
			pathPrefix, locationKey)),
		Index: v.GetStringSlice(fmt.Sprintf("%s.%d.index",
			pathPrefix, locationKey)),
		Rewrite: v.GetString(fmt.Sprintf("%s.%d.rewrite",
			pathPrefix, locationKey)),
		ProxyData: trimProxyPass(v.GetString(fmt.Sprintf("%s.%d.proxy_pass",
			pathPrefix, locationKey))),
		ProxySetHeader: getHeaders(v, fmt.Sprintf("%s.%d.proxy_set_header",
			pathPrefix, locationKey)),
		AppFireWall: v.GetStringSlice(fmt.Sprintf("%s.%d.appfirewall",
			pathPrefix, locationKey)),
		ProxyCache: Cache{
			Path: v.GetString(fmt.Sprintf("%s.%d.proxy_cache.path",
				pathPrefix, locationKey)),
			Valid: v.GetStringSlice(fmt.Sprintf("%s.%d.proxy_cache.valid",
				pathPrefix, locationKey)),
			Key: v.GetString(fmt.Sprintf("%s.%d.proxy_cache.key",
				pathPrefix, locationKey)),
			Methods: v.GetStringSlice(fmt.Sprintf("%s.%d.proxy_cache.methods",
				pathPrefix, locationKey)),
			MaxSize: v.GetInt(fmt.Sprintf("%s.%d.proxy_cache.max_size",
				pathPrefix, locationKey)),
		},
		Limit: PathLimit{
			Size: v.GetInt(fmt.Sprintf("%s.%d.limit.mem",
				pathPrefix, locationKey)),
			Rate: v.GetInt(fmt.Sprintf("%s.%d.limit.rate",
				pathPrefix, locationKey)),
			Burst: v.GetInt(fmt.Sprintf("%s.%d.limit.burst",
				pathPrefix, locationKey)),
			Nodelay: v.GetBool(fmt.Sprintf("%s.%d.limit.nodelay",
				pathPrefix, locationKey)),
		},
		Auth: PathAuth{
			AuthType: v.GetString(fmt.Sprintf("%s.%d.auth.type",
				pathPrefix, locationKey)),
			User: v.GetString(fmt.Sprintf("%s.%d.auth.user",
				pathPrefix, locationKey)),
			Pswd: v.GetString(fmt.Sprintf("%s.%d.auth.pswd",
				pathPrefix, locationKey)),
		},
		ErrorNotFound: v.GetString(fmt.Sprintf("%s.%d.error_page.not_found",
			pathPrefix, locationKey)),
		ErrorProxy: v.GetString(fmt.Sprintf("%s.%d.error_page.proxy",
			pathPrefix, locationKey)),
		Zip: processHttpServerZip(v, pathPrefix, locationKey),
		PathType: processHttpServerPathType(v, pathPrefix, locationKey,
			v.GetString(fmt.Sprintf("%s.%d.proxy_pass", pathPrefix, locationKey))),
		Trys: processTry(v, pathPrefix, locationKey),
	}
}

func processHttpServerZip(v *viper.Viper, pathPrefix string, locationKey int) uint16 {
	var zipType uint16 = ZIP_NONE
	TempZip := v.GetStringSlice(fmt.Sprintf("%s.%d.zip", pathPrefix, locationKey))
	if len(TempZip) > 0 {
		if len(TempZip) == 1 {
			if TempZip[0] == "br" {
				zipType = ZIP_BR
			}
			if TempZip[0] == "gzip" {
				zipType = ZIP_GZIP
			}
		} else if len(TempZip) == 2 {
			if TempZip[0] == "gzip" && TempZip[1] == "br" {
				zipType = ZIP_GZIP_BR
			}
			if TempZip[1] == "gzip" && TempZip[0] == "br" {
				zipType = ZIP_GZIP_BR
			}
		}

	}
	return zipType
}

func processHttpServerPathType(v *viper.Viper, pathPrefix string, locationKey int, proxyData string) uint16 {
	var pathType uint16 = LOCAL
	TempPathType := v.GetString(fmt.Sprintf("%s.%d.type", pathPrefix, locationKey))
	if TempPathType == "local" {
		pathType = LOCAL
	}
	if TempPathType == "rewrite" {
		pathType = REWRITE
	}
	if TempPathType == "devmod" {
		pathType = DEVMOD
	}
	if TempPathType == "proxy" {
		colonIndex := strings.Index(proxyData, ":")
		substring := proxyData[:colonIndex]
		if substring == "http" {
			pathType = PROXY_HTTP
		}
		if substring == "https" {
			pathType = PROXY_HTTPS
		}
	}
	return pathType
}

func processTry(v *viper.Viper, pathPrefix string, locationKey int) []Try {
	var trys []Try
	tryKeys := v.GetStringSlice(fmt.Sprintf("%s.%d.try", pathPrefix, locationKey))

	for tryKey := range tryKeys {
		try := Try{
			Uri:   v.GetString(fmt.Sprintf("%s.%d.try.%d.uri", pathPrefix, locationKey, tryKey)),
			Files: v.GetStringSlice(fmt.Sprintf("%s.%d.try.%d.file", pathPrefix, locationKey, tryKey)),
			Next:  v.GetString(fmt.Sprintf("%s.%d.try.%d.next", pathPrefix, locationKey, tryKey)),
		}
		trys = append(trys, try)
	}
	return trys
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
