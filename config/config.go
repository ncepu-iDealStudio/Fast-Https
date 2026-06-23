package config

import (
	"encoding/json"
	"errors"
	"fast-https/utils/files"
	"fast-https/utils/logger"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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
	MaxSize int // 1023MB
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
	LogSplit                  string
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

// Define Configuration Structure
var GConfig Fast_Https
var GContentTypeMap map[string]string
var GOs = runtime.GOOS

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
	err := processRoot()
	if err != nil {
		return err
	}
	err = serverContentType()
	if err != nil {
		return err
	}
	return nil
}

func Reload() {
	ClearConfig()
	Init()
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
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(rootBaseDir, includePath)
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
	if filepath.IsAbs(path) {
		_, err := os.Stat(path)
		return err == nil
	}

	candidates := []string{
		filepath.Join(baseDir, path),
		filepath.Join(rootBaseDir, path),
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
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

func ClearConfig() {
	GConfig = Fast_Https{}
	GContentTypeMap = map[string]string{}
}

// content types of server
func serverContentType() error {

	GContentTypeMap = make(map[string]string)

	// wd, _ := os.Getwd()
	// confPath := filepath.Join(wd, MIME_FILE_PATH)
	// fmt.Println(MIME_FILE_PATH)
	confBytes, err := files.ReadFile(MIME_FILE_PATH)

	if err != nil {
		logger.Fatal("can't open mime.types file")
		return errors.New("can't open mime.types file")
	}

	err = json.Unmarshal(confBytes, &GContentTypeMap)
	if err != nil {
		logger.Fatal("can't unmarshal mime.json file")
		return errors.New("can't unmarshal mime.json file")
	}

	return nil
}

func processRoot() error {
	rootViper.SetConfigFile(CONFIG_FILE_PATH)

	err := rootViper.ReadInConfig()
	if err != nil {
		logger.Fatal("Error reading config file: %s", err)
	}

	var config Fast_Https
	err = rootViper.Unmarshal(&config)
	if err != nil {
		logger.Fatal("Error unmarshaling config: %s", err)
	}

	GConfig.Pid = rootViper.GetString("pid")
	GConfig.LogRoot = rootViper.GetString("log_root")
	processEngine()
	processHttp()
	SetDefault()
	return nil
}

func processEngine() error {
	GConfig.ServerEngine = Engine{
		IsMaster:     rootViper.GetBool("engine.is_master"),
		Id:           rootViper.GetInt("engine.id"),
		RegisterPort: rootViper.GetInt("engine.register_port"),
		SlaveIp:      rootViper.GetString("engine.slave_ip"),
		SlavePort:    rootViper.GetInt("engine.slave_port"),
	}
	return nil
}

func processHttp() error {
	GConfig.Include = rootViper.GetStringSlice("http.include")
	GConfig.DefaultType = rootViper.GetString("http.default_type")
	GConfig.Limit = ServerLimit{
		MaxHeaderSize: rootViper.GetInt("http.servers_limit.max_header_size"),
		MaxBodySize:   rootViper.GetInt("http.servers_limit.max_body_size"),
		Rate:          rootViper.GetInt("http.servers_limit.limit"),
		Burst:         rootViper.GetInt("http.servers_limit.burst"),
	}

	GConfig.BlackList = rootViper.GetStringSlice("http.blaklist")
	GConfig.LogFormat = rootViper.GetStringSlice("http.log_format")
	GConfig.LogSplit = rootViper.GetString("http.log_split")
	processHttpServer("http.server")
	processIncludeCfg()
	return nil
}

func processIncludeCfg() {
	for _, item := range GConfig.Include {

		err := filepath.Walk(item, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// 跳过目录 "." 和 ".."
			if info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}

			logger.Info("Include config dir: %s", path)
			incViper := viper.New()
			incViper.SetConfigFile(path)
			err = incViper.ReadInConfig()
			if err != nil {
				logger.Fatal("Error reading config file: %s", err)
			}
			var config Fast_Https
			err = incViper.Unmarshal(&config)
			if err != nil {
				logger.Fatal("Error unmarshaling config: %s", err)
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

			GConfig.Servers = append(GConfig.Servers, server)
			return nil
		})
		if err != nil {
			logger.Fatal("processIncludeCfg can not walk")
		}
	}
}

func processHttpServer(pathPrefix string) error {
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
	GConfig.Servers = servers
	return nil
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
			Nodelay: v.GetBool(fmt.Sprintf("%s.%d.limit.mem",
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

func SetDefault() {
	if GConfig.Limit.MaxHeaderSize == 0 {
		GConfig.Limit.MaxHeaderSize = DEFAULT_MAX_HEADER_SIZE
	}
	if GConfig.Limit.MaxBodySize == 0 {
		GConfig.Limit.MaxBodySize = DEFAULT_MAX_BODY_SIZE
	}
	if GConfig.DefaultType == "" {
		GConfig.DefaultType = HTTP_DEFAULT_CONTENT_TYPE
	}

	if GConfig.LogRoot == "" {
		GConfig.LogRoot = DEFAULT_LOG_ROOT
	}
}
