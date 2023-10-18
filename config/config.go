package config

import (
	"errors"
	"fast-https/utils/files"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
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

type ErrorPath struct {
	Path404 string
	Path500 string
}

type Header struct {
	HeaderKey   uint16
	HeaderValue string
}

type Cache struct {
	Path    string
	Valid   []string
	Key     string
	MaxSize int // 1023MB
}

type Path struct {
	PathName       string
	PathType       uint16
	Zip            uint16
	Root           string
	Index          []string
	Rewrite        string
	ProxyData      string
	ProxySetHeader []Header
	ProxyCache     Cache
}

type Server struct {
	Listen            string
	ServerName        string
	SSLCertificate    string
	SSLCertificateKey string
	Path              []Path
}

type Fast_Https struct {
	ErrorPage                 ErrorPath
	Error_log                 string
	Pid                       string
	LogRoot                   string
	Servers                   []Server
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

func getHeaders(path string) []Header {
	headerKeys := viper.GetStringSlice(path)
	var headers []Header
	for headerKey, _ := range headerKeys {
		header := Header{
			HeaderKey:   viper.GetUint16(fmt.Sprintf("%s.%d.HeaderKey", path, headerKey)),
			HeaderValue: viper.GetString(fmt.Sprintf("%s.%d.HeaderValue", path, headerKey)),
		}
		headers = append(headers, header)
	}
	return headers
}

// Define Configuration Structure
var GConfig Fast_Https
var GContentTypeMap map[string]string
var GOs = runtime.GOOS

// Init the whole config module
func Init() error {
	err := process()
	if err != nil {
		return err
	}
	err = serverContentType()
	if err != nil {
		return err
	}
	return nil
}

// CheckConfig check whether config is correct
func CheckConfig() error {
	err := Init()
	if err != nil {
		return err
	}
	return nil
}

func ClearConfig() {
	GConfig = Fast_Https{}
	GContentTypeMap = map[string]string{}
}

// content types of server
func serverContentType() error {

	GContentTypeMap = make(map[string]string)
	var content_type string

	wd, _ := os.Getwd()
	confPath := filepath.Join(wd, "config/mime.types")
	confBytes, err := files.ReadFile(confPath)

	if err != nil {
		return errors.New("can't open mime.types file")
	}
	var clear_str string
	if GOs == "windows" {
		clear_str = strings.ReplaceAll(string(confBytes), "\r\n", "")
	} else {
		clear_str = strings.ReplaceAll(string(confBytes), "\n", "")
	}
	all_type_arr := strings.Split(deleteExtraSpace(clear_str), ";")
	for _, one_type := range all_type_arr {
		arr := strings.Split(one_type, " ")

		for i := 0; i < len(arr); i++ {
			if i == 0 {
				content_type = arr[0]
			} else {
				GContentTypeMap[arr[i]] = content_type
			}
		}

	}
	return nil
}

func deleteExtraSpace(s string) string {
	//Remove excess spaces from the string, and when there are multiple spaces, only one space is retained
	s1 := strings.Replace(s, "	", " ", -1)       //Replace tab with a space
	regstr := "\\s{2,}"                          //Regular expressions with two or more spaces
	reg, _ := regexp.Compile(regstr)             //Compiling Regular Expressions
	s2 := make([]byte, len(s1))                  //Define character array slicing
	copy(s2, s1)                                 //Copy String to Slice
	spc_index := reg.FindStringIndex(string(s2)) //Search in strings
	for len(spc_index) > 0 {                     //Find Adapt
		s2 = append(s2[:spc_index[0]+1], s2[spc_index[1]:]...) //Remove excess spaces
		spc_index = reg.FindStringIndex(string(s2))            //Continue searching in strings
	}
	return string(s2)
}

func process() error {

	var fast_https Fast_Https

	viper.SetConfigFile("config/fast-https.json") // 指定要解析的 JSON 文件

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatal("Error reading config file:", err)
	}

	var config Fast_Https
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Fatal("Error unmarshaling config:", err)
	}

	fast_https.Error_log = viper.GetString("error_log")
	fast_https.Pid = viper.GetString("pid")
	fast_https.Include = viper.GetStringSlice("http.include")
	fast_https.DefaultType = viper.GetString("http.default_type")

	var servers []Server

	// 遍历每个服务器块，并解析为 Server 结构体
	serverKeys := viper.GetStringSlice("http.server")
	for serverKey, _ := range serverKeys {
		server := Server{
			Listen:            viper.GetString(fmt.Sprintf("http.server.%d.listen", serverKey)),
			ServerName:        viper.GetString(fmt.Sprintf("http.server.%d.server_name", serverKey)),
			SSLCertificate:    viper.GetString(fmt.Sprintf("http.server.%d.ssl_certificate", serverKey)),
			SSLCertificateKey: viper.GetString(fmt.Sprintf("http.server.%d.ssl_certificate_key", serverKey)),
		}

		pathPrefix := fmt.Sprintf("http.server.%d.location", serverKey)
		locationKeys := viper.GetStringSlice(pathPrefix)

		var paths []Path
		for locationKey, _ := range locationKeys {
			path := Path{

				PathName: viper.GetString(fmt.Sprintf("%s.%d.url", pathPrefix, locationKey)),
				//PathType:       viper.GetUint16(fmt.Sprintf("%s.%d.path_type", pathPrefix, locationKey)),
				//Zip:            viper.GetUint16(fmt.Sprintf("%s.%d.zip", pathPrefix, locationKey)),
				Root:           viper.GetString(fmt.Sprintf("%s.%d.root", pathPrefix, locationKey)),
				Index:          viper.GetStringSlice(fmt.Sprintf("%s.%d.index", pathPrefix, locationKey)),
				Rewrite:        viper.GetString(fmt.Sprintf("%s.%d.rewrite", pathPrefix, locationKey)),
				ProxyData:      viper.GetString(fmt.Sprintf("%s.%d.proxy_pass", pathPrefix, locationKey)),
				ProxySetHeader: getHeaders(fmt.Sprintf("%s.%d.proxy_set_header", pathPrefix, locationKey)),
				ProxyCache: Cache{
					Path:    viper.GetString(fmt.Sprintf("%s.%d.proxy_cache.path", pathPrefix, locationKey)),
					Valid:   viper.GetStringSlice(fmt.Sprintf("%s.%d.proxy_cache.valid", pathPrefix, locationKey)),
					Key:     viper.GetString(fmt.Sprintf("%s.%d.proxy_cache.key", pathPrefix, locationKey)),
					MaxSize: viper.GetInt(fmt.Sprintf("%s.%d.proxy_cache.max_size", pathPrefix, locationKey)),
				},
			}
			TempZip := viper.GetStringSlice(fmt.Sprintf("%s.%d.zip", pathPrefix, locationKey))
			if len(TempZip) > 0 {
				if len(TempZip) == 1 {
					if TempZip[0] == "br" {
						path.Zip = ZIP_BR
					}
					if TempZip[0] == "gzip" {
						path.Zip = ZIP_GZIP
					}
				} else if len(TempZip) == 2 {
					if TempZip[0] == "gzip" && TempZip[1] == "br" {
						path.Zip = ZIP_GZIP_BR
					}
					if TempZip[1] == "gzip" && TempZip[0] == "br" {
						path.Zip = ZIP_GZIP_BR
					}
				}

			}

			TempPathType := viper.GetString(fmt.Sprintf("%s.%d.type", pathPrefix, locationKey))
			if TempPathType == "local" {
				path.PathType = 0
			}
			if TempPathType == "rewrite" {
				path.PathType = 4
			}
			if TempPathType == "proxy" {
				TempProxyData := path.ProxyData
				colonIndex := 0
				for i, char := range TempProxyData {
					if char == ':' {
						colonIndex = i
						break
					}
				}
				substring := TempProxyData[:colonIndex]
				if substring == "http" {
					path.PathType = 1
				}
				if substring == "https" {
					path.PathType = 2
				}
				path.ProxyData = TempProxyData[colonIndex+3:]
			}

			paths = append(paths, path)
		}
		server.Path = paths

		servers = append(servers, server)
	}
	fast_https.Servers = servers

	// fmt.Println(fast_https)
	GConfig = fast_https

	return nil

}
