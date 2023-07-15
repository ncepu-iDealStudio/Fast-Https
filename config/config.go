package config

import (
	"fast-https/utils/files"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type static struct {
	Root  string
	Index []string
}

type ErrorPath struct {
	Code uint8
	Path string
}

type HttpServer struct {
	Listen     string
	ServerName string
	Static     static
	Path       string
	Ssl        string
	Ssl_Key    string
	Gzip       uint8

	PROXY_TYPE uint8
	PROXY_DATA string
}

type Config struct {
	ErrorPage  ErrorPath
	LogRoot    string
	HttpServer []HttpServer
}

// 定义配置结构体
var G_config Config
var G_ContentTypeMap map[string]string

func init() {
	fmt.Println("-----[Fast-Https]config init...")
	process()
	ServerContentType()
}

func ServerContentType() {

	G_ContentTypeMap = make(map[string]string)
	var content_type string

	confBytes, err := files.ReadFile("./config/mime.types")
	if err != nil {
		log.Fatal("Can't open mime.types file")
	}

	clear_str := strings.ReplaceAll(string(confBytes), "\n", "")
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

func delete_extra_space(s string) string {
	//删除字符串中的多余空格，有多个空格时，仅保留一个空格
	s1 := strings.Replace(s, "	", " ", -1)       //替换tab为空格
	regstr := "\\s{2,}"                          //两个及两个以上空格的正则表达式
	reg, _ := regexp.Compile(regstr)             //编译正则表达式
	s2 := make([]byte, len(s1))                  //定义字符数组切片
	copy(s2, s1)                                 //将字符串复制到切片
	spc_index := reg.FindStringIndex(string(s2)) //在字符串中搜索
	for len(spc_index) > 0 {                     //找到适配项
		s2 = append(s2[:spc_index[0]+1], s2[spc_index[1]:]...) //删除多余空格
		spc_index = reg.FindStringIndex(string(s2))            //继续在字符串中搜索
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

func process() {
	content, err := os.ReadFile("./config/fast-https.conf")
	if err != nil {
		fmt.Println("读取配置文件失败：", err)
		return
	}

	//删除注释
	clear_str := ""
	for _, line := range strings.Split(string(content), "\n") {
		clear_str += delete_comment(line) + "\n"
	}

	// 检查是否存在include语句
	includeRe := regexp.MustCompile(`include\s+([^;]+);`)
	matches := includeRe.FindAllStringSubmatch(clear_str, -1)
	if matches != nil {
		for _, match := range matches {
			includePath := strings.TrimSpace(match[1])

			// 扩展include语句
			expandedPaths := expandInclude(includePath)

			// 逐个读取扩展后的配置文件
			for _, path := range expandedPaths {
				fileContent, err := os.ReadFile(path)
				if err != nil {
					fmt.Println("读取配置文件失败：", err)
					continue
				}

				// 将扩展后的配置文件内容拼接到clear_str中，用于后续解析
				clear_str += string(fileContent) + "\n"
			}
		}
	}

	// 定义正则表达式
	pattern := `server\s*{([^}]*)}`
	re := regexp.MustCompile(pattern)

	// 使用正则表达式解析出所有 server 块内容
	matches = re.FindAllStringSubmatch(clear_str, -1)
	if matches == nil {
		fmt.Println("没有找到 server 块")
		return
	}

	// 循环遍历每个 server 块
	for _, match := range matches {

		// 定义 HttpServer 结构体
		var server HttpServer

		// 解析 server_name 字段
		re = regexp.MustCompile(`server_name\s+([^;]+);`)
		serverName := re.FindStringSubmatch(match[1])
		if len(serverName) > 1 {
			server.ServerName = strings.TrimSpace(serverName[1])
		}

		// 解析 listen 字段
		re = regexp.MustCompile(`listen\s+([^;]+);`)
		listen := re.FindStringSubmatch(match[1])
		if len(listen) > 1 {
			server.Listen = strings.TrimSpace(listen[1])
		}

		//解析Gzip
		re = regexp.MustCompile(`Gzip\s+([^;]+);`)
		gzip := re.FindStringSubmatch(match[1])
		if len(gzip) > 1 {
			server.Gzip = 1
		}

		// 解析 ssl 和 ssl_key 字段
		re = regexp.MustCompile(`ssl\s+([^;]+);`)
		ssl := re.FindStringSubmatch(match[1])
		if len(ssl) > 1 {
			server.Ssl = strings.TrimSpace(ssl[1])
		}

		re = regexp.MustCompile(`ssl_key\s+([^;]+);`)
		sslKey := re.FindStringSubmatch(match[1])
		if len(sslKey) > 1 {
			server.Ssl_Key = strings.TrimSpace(sslKey[1])
		}

		// 解析 path 字段和static字段
		re = regexp.MustCompile(`path\s+(/[^{]+)`)
		path := re.FindStringSubmatch(match[1])
		if len(path) > 1 {
			server.Path = strings.TrimSpace(path[1])
		}

		re = regexp.MustCompile(`path\s+/[^{]+{[^}]*root\s+([^;]+);[^}]*index\s+([^;]+);`)
		staticMatches := re.FindStringSubmatch(match[1])
		if len(staticMatches) > 2 {
			server.Static.Root = strings.TrimSpace(staticMatches[1])
			server.Static.Index = parseIndex(strings.TrimSpace(staticMatches[2]))
		}

		// 解析 TCP_PROXY 和 HTTP_PROXY 字段
		re = regexp.MustCompile(`TCP_PROXY\s+([^;]+);`)
		tcpProxy := re.FindStringSubmatch(match[1])

		re = regexp.MustCompile(`HTTP_PROXY\s+([^;]+);`)
		httpProxy := re.FindStringSubmatch(match[1])
		if len(httpProxy) > 1 {
			server.PROXY_TYPE = 1
			server.PROXY_DATA = strings.TrimSpace(httpProxy[1])
		}

		re = regexp.MustCompile(`HTTPS_PROXY\s+([^;]+);`)
		httpsProxy := re.FindStringSubmatch(match[1])
		if len(httpsProxy) > 1 {
			server.PROXY_TYPE = 2
			server.PROXY_DATA = strings.TrimSpace(httpsProxy[1])
		}

		if len(tcpProxy) > 1 {
			server.PROXY_TYPE = 3
			server.PROXY_DATA = strings.TrimSpace(tcpProxy[1])
		}

		// 将解析出的 HttpServer 结构体添加到 Config 结构体中
		G_config.HttpServer = append(G_config.HttpServer, server)
	}

	// 解析 error_page 字段

	re = regexp.MustCompile(`error_page\s+(\d+)\s+([^;]+);`)
	errorPage := re.FindStringSubmatch(string(content))
	if len(errorPage) > 1 {
		//config.ErrorPage.Code = uint8(errorPage[1])
		temp, _ := strconv.Atoi(errorPage[1])
		G_config.ErrorPage.Code = uint8(temp)
		G_config.ErrorPage.Path = strings.TrimSpace(errorPage[2])
	}
	//re = regexp.MustCompile(`error_page\s+(\d+)\s+([^;]+);`)
	//errorPage := re.FindStringSubmatch(string(content))
	//if len(errorPage) > 1 {
	//	code, err := strconv.ParseUint(errorPage[1], 10, 8)
	//	if err != nil {
	//		fmt.Println("解析 error_page 中的 code 字段失败：", err)
	//		return
	//	}
	//	config.ErrorPage.Code = uint8(code)
	//	config.ErrorPage.Path = strings.TrimSpace(errorPage[2])
	//}

	// 解析 log_root 字段
	re = regexp.MustCompile(`log_root\s+([^;]+);`)
	logRoot := re.FindStringSubmatch(string(content))
	if len(logRoot) > 1 {
		G_config.LogRoot = strings.TrimSpace(logRoot[1])
	}
	fmt.Println(G_config)

	// fmt.Printf("%+v\n", G_config)
}
