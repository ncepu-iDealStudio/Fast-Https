package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"strings"
)

// var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"
const (
	HTTP_DEFAULT_CONTENT_TYPE = "text/html"
)

func StaticEvent(lisdata listener.ListenData, path string) []byte {
	if config.G_OS == "windows" {
		path = "/" + path
	}
	var res []byte
	var file_data = cache.Get_data_from_cache(path)

	if file_data != nil { // Not Fount

		response := response.Response_init()
		response.Set_first_line(200, "OK")
		response.Set_header("Server", "Fast-Https")
		response.Set_header("Content-Type", get_content_type(path))
		if lisdata.Zip == 1 {
			response.Set_header("Content-Encoding", "gzip")
		}
		response.Set_body([]byte("<h1>Hello, World!</h1>"))
		return response.Generate_response()
	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + "/" + item
		realPath = strings.ReplaceAll(realPath, "//", "/")
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			response := response.Response_init()
			response.Set_first_line(200, "OK")
			response.Set_header("Server", "Fast-Https")
			response.Set_header("Content-Type", get_content_type(path))
			if lisdata.Zip == 1 {
				response.Set_header("Content-Encoding", "gzip")
			}
			response.Set_body([]byte("<h1>Hello, World!</h1>"))
			return response.Generate_response()
		}
	}

	res = response.Default_not_found

	return res
}

func get_content_type(path string) string {
	path_type := strings.Split(path, ".")

	if path_type == nil {
		return HTTP_DEFAULT_CONTENT_TYPE
	}
	pointAfter := path_type[len(path_type)-1]
	row := config.G_ContentTypeMap[pointAfter]
	if row == "" {
		sep := "?"
		index := strings.Index(pointAfter, sep)
		if index != -1 { // 如果存在特定字符
			pointAfter = pointAfter[:index] // 删除特定字符之后的所有字符
		}
		secondFind := config.G_ContentTypeMap[pointAfter]
		if secondFind != "" {
			return secondFind
		} else {
			return HTTP_DEFAULT_CONTENT_TYPE
		}
	}
	return row
}
