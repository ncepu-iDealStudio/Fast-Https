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

// Define Configuration Structure
var G_config Config
var G_ContentTypeMap map[string]string
var G_OS = ""

func init() {
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

	confBytes, err := files.ReadFile("config/mime.types")
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

func process() {
	content, err := os.ReadFile("./config/fast-https.conf")
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
	pattern := `server\s*{([^}]*)}`
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
		var server HttpServer

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

		//Parsing Gzip
		re = regexp.MustCompile(`gzip\s+([^;]+);`)
		gzip := re.FindStringSubmatch(match[1])
		if len(gzip) > 1 {
			server.Gzip = 1
		}

		// Parsing SSL and SSL_ Key field
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

		//Parsing path and static fields
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

		// Parsing TCP_ PROXY and HTTP_ PROXY field
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

		// Add the parsed HttpServer structure to the Config structure
		G_config.HttpServer = append(G_config.HttpServer, server)
	}

	// Parse error_ Page field

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
	//		fmt.Println("Parse error_ Code field in page failed:", err)
	//		return
	//	}
	//	config.ErrorPage.Code = uint8(code)
	//	config.ErrorPage.Path = strings.TrimSpace(errorPage[2])
	//}

	// Parsing logs_ Root field
	re = regexp.MustCompile(`log_root\s+([^;]+);`)
	logRoot := re.FindStringSubmatch(string(content))
	if len(logRoot) > 1 {
		G_config.LogRoot = strings.TrimSpace(logRoot[1])
	}
	// fmt.Println(G_config)

	// fmt.Printf("%+v\n", G_config)
}
