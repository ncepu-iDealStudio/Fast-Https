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
// TODO: check json confgure
func CheckConfig() error {
	return nil
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
			if info.IsDir() || !strings.Contains(path, "json") {
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
