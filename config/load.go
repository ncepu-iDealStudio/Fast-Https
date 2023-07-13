package config

import (
	"fmt"
	"io/ioutil"
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

func Process() {
	content, err := ioutil.ReadFile("./config/fast-https.conf")
	if err != nil {
		fmt.Println("读取配置文件失败：", err)
		return
	}

	// 定义正则表达式
	pattern := `server\s*{([^}]*)}`
	re := regexp.MustCompile(pattern)

	// 使用正则表达式解析出所有 server 块内容
	matches := re.FindAllStringSubmatch(string(content), -1)
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

		//rePath := regexp.MustCompile(`path\s+\/(.+?)\s+\{`)
		//
		//lines := strings.Split(string(content), "\n")
		//	for i := 0; i < len(lines); i++ {
		//	line := strings.TrimSpace(lines[i])
		//	if line == "" || strings.HasPrefix(line, "#") {
		//		continue
		//	}
		//	if matches := rePath.FindStringSubmatch(line); len(matches) > 0 {
		//		server.Path = "/" + matches[1]
		//
		//	}
		//
		//}

		re = regexp.MustCompile(`path\s+/[^{]+{[^}]*root\s+([^;]+);[^}]*index\s+([^;]+);`)
		staticMatches := re.FindStringSubmatch(match[1])
		if len(staticMatches) > 2 {
			server.Static.Root = strings.TrimSpace(staticMatches[1])
			server.Static.Index = parseIndex(strings.TrimSpace(staticMatches[2]))
		}

		//
		//re = regexp.MustCompile(`path\s+([^{}]+)\s*{([^}]*)}`)
		//path := re.FindStringSubmatch(match[1])
		//if len(path) > 2 {
		//	server.Path = strings.TrimSpace(path[1])
		//
		//	// 解析 path 块中的字段
		//	re = regexp.MustCompile(`root\s+([^;]+);`)
		//	root := re.FindStringSubmatch(path[2])
		//	if len(root) > 1 {
		//		server.Static.Root = strings.TrimSpace(root[1])
		//	}
		//
		//	re = regexp.MustCompile(`index\s+([^;]+);`)
		//	index := re.FindStringSubmatch(path[2])
		//	if len(index) > 1 {
		//		server.Static.Index = strings.Fields(strings.TrimSpace(index[1]))
		//	}
		//}

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

	// 打印解析后的配置信息
	fmt.Printf("%+v\n", G_config)
}

<<<<<<< HEAD
//func main() {
//	Process()
//}

=======
>>>>>>> 4e5114c7748c7764fada2154ed949d688d737b99
// Open the config file for reading
//file, err := os.Open("config.conf")
//if err != nil {
//	fmt.Println("Error opening file:", err)
//	return
//}
//defer file.Close()

// Declare a new Config struct
//config := Config{}

// Define regular expressions to match the config file syntax
//	workerProcessesRegex := regexp.MustCompile(`^worker_processes\s+(\d+)\s*;\s*$`)
//	eventsWorkerConnectionsRegex := regexp.MustCompile(`^worker_connections\s+(\d+)\s*;\s*$`)
//	httpClientMaxBodySizeRegex := regexp.MustCompile(`^client_max_body_size\s+(\S+)\s*;\s*$`)
//	httpServerRegex := regexp.MustCompile(`^server\s*\{\s*$`)
//	listenRegex := regexp.MustCompile(`^listen\s+(\S+)\s*;\s*$`)
//	serverNameRegex := regexp.MustCompile(`^server_name\s+(\S+)\s*;\s*$`)
//	rootRegex := regexp.MustCompile(`^root\s+(\S+)\s*;\s*$`)
//	indexRegex := regexp.MustCompile(`^index\s+(\S+)\s*;\s*$`)
//	errorPageRegex := regexp.MustCompile(`^error_page\s+(.*)\s*;\s*$`)
//	locationRegex := regexp.MustCompile(`^location\s+(\S+)\s*\{\s*$`)
//
//	// Create a scanner to read the file line by line
//	scanner := bufio.NewScanner(file)
//
//	// Loop through each line of the file
//	for scanner.Scan() {
//		line := scanner.Text()
//
//		// Ignore lines that start with #
//		if strings.HasPrefix(strings.TrimSpace(line), "#") {
//			continue
//		}
//
//		// Check if the line matches the syntax for the worker_processes property
//		if matches := workerProcessesRegex.FindStringSubmatch(line); len(matches) > 0 {
//			// Store the value in the config struct
//			config.WorkerProcesses, _ = strconv.Atoi(matches[1])
//			continue
//		}
//
//		// Check if the line matches the syntax for the worker_connections property
//		if matches := eventsWorkerConnectionsRegex.FindStringSubmatch(line); len(matches) > 0 {
//			// Store the value in the config struct
//			config.EventsWorkerConnections, _ = strconv.Atoi(matches[1])
//			continue
//		}
//
//		// Check if the line matches the syntax for the client_max_body_size property
//		if matches := httpClientMaxBodySizeRegex.FindStringSubmatch(line); len(matches) > 0 {
//			// Store the value in the config struct
//			config.HttpClientMaxBodySize = matches[1]
//			continue
//		}
//
//		// Check if the line matches the syntax for the http server block
//		if matches := httpServerRegex.FindStringSubmatch(line); len(matches) > 0 {
//			// Initialize the httpServer struct
//			httpServer := &config.HttpServer{}
//
//			// Loop through the lines until the end of the server block
//			for scanner.Scan() {
//				line := scanner.Text()
//
//				// Check if the line matches the syntax for the listen property
//				if matches := listenRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.Listen = matches[1]
//					continue
//				}
//
//				// Check if the line matches the syntax for the server_name property
//				if matches := serverNameRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.ServerName = matches[1]
//					continue
//				}
//
//				// Check if the line matches the syntax for the root property
//				if matches := rootRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.Root = matches[1]
//					continue
//				}
//
//				// Check if the line matches the syntax for the index property
//				if matches := indexRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.Index = matches[1]
//					continue
//				}
//
//				// Check if the line matches the syntax for the error_page property
//				if matches := errorPageRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.ErrorPage = matches[1]
//					continue
//				}
//
//				// Check if the line matches the syntax for the location block
//				if matches := locationRegex.FindStringSubmatch(line); len(matches) > 0 {
//					// Store the value in the httpServer struct
//					httpServer.Location = matches[1]
//
//					// Loop through the lines until the end of the location block
//					for scanner.Scan() {
//						locationLine := scanner.Text()
//
//						// Check if the line ends the location block
//						if strings.TrimSpace(locationLine) == "}" {
//							break
//						}
//					}
//
//					continue
//				}
//
//				// Check if the line ends the server block
//				if strings.TrimSpace(line) == "}" {
//					break
//				}
//			}
//
//			// Store the httpServer struct in the config struct
//			config.HttpServer = *httpServer
//
//			continue
//		}
//	}
//
//	// Print out the values for testing purposes
//	fmt.Println("Worker processes:", config.WorkerProcesses)
//	fmt.Println("Events worker connections:", config.EventsWorkerConnections)
//	fmt.Println("HTTP client max body size:", config.HttpClientMaxBodySize)
//	fmt.Println("HTTP server listen:", config.HttpServer.Listen)
//	fmt.Println("HTTP server name:", config.HttpServer.ServerName)
//	fmt.Println("HTTP server root:", config.HttpServer.Root)
//	fmt.Println("HTTP server index:", config.HttpServer.Index)
//	fmt.Println("HTTP server error page:", config.HttpServer.ErrorPage)
//	fmt.Println("HTTP server location:", config.HttpServer.Location)
//}
