package config

import (
	"fast-https/utils/files"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const (
	HTTP_DEFAULT_CONTENT_TYPE = "text/html"
)

const (
	ZIP_NONE    = 0
	ZIP_GZIP    = 1
	ZIP_BR      = 2
	ZIP_GZIP_BR = 10
)

type global struct {
	workerProcesses uint8
}

type events struct {
	eventDrivenModel  string
	workerConnections uint8
}

type ErrorPath struct {
	Code uint8
	Path string
}

type Path struct {
	PathName                    string
	PathType                    uint8
	Zip                         uint8
	Root                        string // static file  pathtype = 0
	Index                       []string
	Rewrite                     string // rewrite	   pathtype = 4
	ProxyData                   string // Proxy	       pathtype = 1, 2
	ProxySetHeaderHost          string
	ProxySetHeaderXRealIp       string
	ProxySetHeaderXForwardedFor string
	Deny                        string
	Allow                       string
}

type Server struct {
	Listen            string
	ServerName        string
	SSLCertificate    string
	SSLCertificateKey string
	Path              []Path
}

type Fast_Https struct {
	ErrorPage ErrorPath
	LogRoot   string
	Servers   []Server

	Include                   string // 需要包含的文件映射类型
	DefaultType               string // 默认文件类型配置
	ServerNamesHashBucketSize uint8  // 服务器名字的hash表大小
	ClientHeaderBufferSize    uint8  // 上传文件大小限制
	LargeClientHeaderBuffers  uint8  // 设定请求缓
	ClientMaxBodySize         uint8  // 设定请求缓
	KeepaliveTimeout          uint8  // 连接超时时间，默认为75s，可以在http，server，location块。
	AutoIndex                 string // 显示目录
	AutoIndexExactSize        string // 显示文件大小 默认为on,显示出文件的确切大小,单位是bytes 改为off后,显示出文件的大概大小,单位是kB或者MB或者GB
	AutoIndexLocaltime        string // 显示文件时间 默认为off,显示的文件时间为GMT时间 改为on后,显示的文件时间为文件的服务器时间
	Sendfile                  string // 开启高效文件传输模式,sendfile指令指定nginx是否调用sendfile函数来输出文件,对于普通应用设为 on,如果用来进行下载等应用磁盘IO重负载应用,可设置为off,以平衡磁盘与网络I/O处理速度,降低系统的负载.注意：如果图片显示不正常把这个改成off.
	TcpNopush                 string // 防止网络阻塞
	TcpNodelay                string // 防止网络阻塞

}

// Define Configuration Structure
var G_config Fast_Https
var G_ContentTypeMap map[string]string
var G_OS = ""

func Init() {
	if runtime.GOOS == "linux" {
		G_OS = "linux"
	} else {
		G_OS = "windows"
	}
	// fmt.Println("-----[Fast-Https]config init...")
	process()
	ServerContentType()

}

func expandInclude(path string) []string {
	// Parse the include statement to obtain the wildcard part
	dir, file := filepath.Split(path)
	dir = filepath.Clean(dir)

	// Find matching files
	matches, err := filepath.Glob(filepath.Join(dir, file))
	if err != nil {
		log.Printf("Unable to parse include statement: %v", err)
		return nil
	}

	return matches
}

func delete_comment(s string) string {
	var sb strings.Builder
	inString := false
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inString = !inString
		}
		if !inString && s[i] == '#' {
			break
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func ServerContentType() {

	G_ContentTypeMap = make(map[string]string)
	var content_type string

	wd, err := os.Getwd()
	confPath := filepath.Join(wd, "config/mime.types")
	confBytes, err := files.ReadFile(confPath)

	if err != nil {
		log.Fatal("Can't open mime.types file")
	}
	var clear_str string
	if G_OS == "windows" {
		clear_str = strings.ReplaceAll(string(confBytes), "\r\n", "")
	} else {
		clear_str = strings.ReplaceAll(string(confBytes), "\n", "")
	}
	all_type_arr := strings.Split(delete_extra_space(clear_str), ";")
	for _, one_type := range all_type_arr {
		arr := strings.Split(one_type, " ")

		for i := 0; i < len(arr); i++ {
			if i == 0 {
				content_type = arr[0]
			} else {
				G_ContentTypeMap[arr[i]] = content_type
			}
		}

	}
}

func GetContentType(path string) string {
	path_type := strings.Split(path, ".")

	if path_type == nil {
		return HTTP_DEFAULT_CONTENT_TYPE
	}
	pointAfter := path_type[len(path_type)-1]
	row := G_ContentTypeMap[pointAfter]
	if row == "" {
		sep := "?"
		index := strings.Index(pointAfter, sep)
		if index != -1 { // 如果存在特定字符
			pointAfter = pointAfter[:index] // 删除特定字符之后的所有字符
		}
		//fmt.Println(pointAfter, "iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii")
		secondFind := G_ContentTypeMap[pointAfter]
		if secondFind != "" {
			return secondFind
		} else {
			return HTTP_DEFAULT_CONTENT_TYPE
		}
	}

	return row

}

func delete_extra_space(s string) string {
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

func parseIndex(indexStr string) []string {
	var index []string
	inString := false
	inBrace := false
	var sb strings.Builder
	for i := 0; i < len(indexStr); i++ {
		switch indexStr[i] {
		case ' ', '\t', '\r', '\n':
			if inString {
				sb.WriteByte(indexStr[i])
			} else if inBrace {
				sb.WriteByte(indexStr[i])
			} else {
				if sb.Len() > 0 {
					index = append(index, sb.String())
					sb.Reset()
				}
			}
		case '{':
			inBrace = true
			sb.WriteByte(indexStr[i])
		case '}':
			inBrace = false
			sb.WriteByte(indexStr[i])
		case '"':
			inString = !inString
			sb.WriteByte(indexStr[i])
		default:
			sb.WriteByte(indexStr[i])
		}
	}
	if sb.Len() > 0 {
		index = append(index, sb.String())
	}
	return index
}

func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

func process() {
	wd, err := os.Getwd()
	confPath := filepath.Join(wd, "config/fast-https-test.conf")
	content, err := os.ReadFile(confPath)
	if err != nil {
		fmt.Println("Failed to read configuration file：", err)
		return
	}

	//Delete Note
	clear_str := ""
	for _, line := range strings.Split(string(content), "\n") {
		clear_str += delete_comment(line) + "\n"
	}

	// Check if there are include statements
	includeRe := regexp.MustCompile(`include\s+([^;]+);`)
	matches := includeRe.FindAllStringSubmatch(clear_str, -1)
	if matches != nil {
		for _, match := range matches {

			includePath := strings.TrimSpace(match[1])

			// Extend include statement
			expandedPaths := expandInclude(includePath)

			// Read the extended configuration files one by one
			for _, path := range expandedPaths {
				fileContent, err := os.ReadFile(path)

				if err != nil {
					fmt.Println("Failed to read configuration file:", err)
					continue
				}

				clear_str_temp := ""
				for _, line := range strings.Split(string(fileContent), "\n") {
					clear_str_temp += delete_comment(line) + "\n"
				}

				// Splice the expanded configuration file content into clear_ In str, for subsequent parsing
				clear_str += clear_str_temp + "\n"
			}
		}
	}

	// Defining Regular Expressions
	pattern := `server\s*{([^{}]*(?:{[^{}]*}[^{}]*)*)}`
	re := regexp.MustCompile(pattern)

	// Parse all server block contents using regular expressions
	matches = re.FindAllStringSubmatch(clear_str, -1)
	if matches == nil {
		fmt.Println("Server block not found")
		return
	}

	// Loop through each server block

	for _, match := range matches {

		// Define the HttpServer structure
		var server Server

		// Parsing server_ Name field
		re = regexp.MustCompile(`server_name\s+([^;]+);`)
		serverName := re.FindStringSubmatch(match[1])
		if len(serverName) > 1 {
			server.ServerName = strings.TrimSpace(serverName[1])
		}

		// Parsing the listen field
		re = regexp.MustCompile(`listen\s+([^;]+);`)
		listen := re.FindStringSubmatch(match[1])
		if len(listen) > 1 {
			server.Listen = strings.TrimSpace(listen[1])
		}

		// Parsing SSL and SSL_ Key field
		re = regexp.MustCompile(`ssl_certificate\s+([^;]+);`)
		ssl := re.FindStringSubmatch(match[1])
		if len(ssl) > 1 {
			server.SSLCertificate = strings.TrimSpace(ssl[1])
		}

		re = regexp.MustCompile(`ssl_certificate_key\s+([^;]+);`)
		sslKey := re.FindStringSubmatch(match[1])
		if len(sslKey) > 1 {
			server.SSLCertificateKey = strings.TrimSpace(sslKey[1])
		}

		zipRe := regexp.MustCompile(`zip\s+([^;]+);`)
		rootRe := regexp.MustCompile(`root\s+([^;]+)`)
		indexRe := regexp.MustCompile(`index\s+([^;]+)`)
		re = regexp.MustCompile(`path\s+(\S+)\s*{([^}]*)}`)

		zipMatch := zipRe.FindStringSubmatch(match[1])

		server_clear_str := ""
		for _, line := range strings.Split(match[1], "\n") {
			server_clear_str += delete_comment(line) + "\n"
		}

		paths := re.FindAllStringSubmatch(server_clear_str, -1)
		for _, path := range paths {
			var p Path
			p.PathName = strings.TrimSpace(path[1])
			if p.PathName == "" {
				p.PathName = "/"
			}

			if len(zipMatch) > 1 {
				if zipMatch[1] == "gzip br" || zipMatch[1] == "br gzip" {
					p.Zip = 10
				} else if zipMatch[1] == "br" {
					p.Zip = 2
				} else if zipMatch[1] == "gzip" {
					p.Zip = 1
				}
			}

			re = regexp.MustCompile(`proxy_tcp\s+([^;]+)`)
			if len(re.FindStringSubmatch(path[2])) > 1 {
				p.PathType = 3
				p.ProxyData = strings.TrimSpace(re.FindStringSubmatch(path[2])[1])
			}

			re = regexp.MustCompile(`proxy_http\s+([^;]+)`)
			if len(re.FindStringSubmatch(path[2])) > 1 {
				p.PathType = 1
				p.ProxyData = strings.TrimSpace(re.FindStringSubmatch(path[2])[1])
			}

			re = regexp.MustCompile(`proxy_http\s+([^;]+)`)
			if len(re.FindStringSubmatch(path[2])) > 1 {
				p.PathType = 2
				p.ProxyData = strings.TrimSpace(re.FindStringSubmatch(path[2])[1])
			}

			if len(rootRe.FindStringSubmatch(path[2])) > 1 {
				p.Root = strings.TrimSpace(rootRe.FindStringSubmatch(path[2])[1])
			}

			if len(indexRe.FindStringSubmatch(path[2])) > 1 {
				p.Index = strings.Fields(strings.TrimSpace(indexRe.FindStringSubmatch(path[2])[1]))

			}

			server.Path = append(server.Path, p)
		}

		// Parsing TCP_ PROXY and HTTP_ PROXY field

		// Add the parsed HttpServer structure to the Config structure
		G_config.Servers = append(G_config.Servers, server)
	}
	// each server end
	// Parse error_ Page field

	re = regexp.MustCompile(`error_page\s+(\d+)\s+([^;]+);`)
	errorPage := re.FindStringSubmatch(string(content))
	if len(errorPage) >= 1 {
		//config.ErrorPage.Code = uint8(errorPage[1])
		temp, _ := strconv.Atoi(errorPage[1])
		G_config.ErrorPage.Code = uint8(temp)
		G_config.ErrorPage.Path = strings.TrimSpace(errorPage[2])
	}

	re = regexp.MustCompile(`log_root\s+([^;]+);`)
	logRoot := re.FindStringSubmatch(string(content))
	if len(logRoot) >= 1 {
		G_config.LogRoot = strings.TrimSpace(logRoot[1])
	}

	//fmt.Println(G_config.Server[1].Path)
	//fmt.Println(G_config.Server[0].Zip, G_config.Server[1].Zip)

	//fmt.Printf("%+v\n", G_config)
	// pretty.Print(G_config)
}
