package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"strconv"
	"strings"
)

// var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func StaticEvent(lisdata listener.ListenData, path string) []byte {
	if config.G_OS == "windows" {
		path = "/" + path
	}
	var res []byte
	var file_data = cache.Get_data_from_cache(path)

	if file_data != nil { // Not Fount

		head := HTTP_OK + HTTP_SPLIT
		head += FAST_HTTPS + HTTP_SPLIT

		head += "Content-Type: " + config.GetContentType(path) + HTTP_SPLIT
		if lisdata.Zip == 1 {
			head += "Content-Encoding: gzip" + HTTP_SPLIT
		}
		head += "Content-Length: " + strconv.Itoa(len(file_data)) + HTTP_SPLIT
		head += HTTP_SPLIT

		head_byte := []byte(head)
		res = append(res, head_byte...)
		res = append(res, file_data...)
		return res

	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array
		realPath := path + "/" + item
		realPath = strings.ReplaceAll(realPath, "//", "/")
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			head := HTTP_OK + HTTP_SPLIT
			head += FAST_HTTPS + HTTP_SPLIT

			head += "Content-Type: " + config.GetContentType(path) + HTTP_SPLIT
			if lisdata.Zip == 1 {
				head += "Content-Encoding: gzip" + HTTP_SPLIT
			}
			head += "Content-Length: " + strconv.Itoa(len(file_data)) + HTTP_SPLIT
			head += HTTP_SPLIT

			head_byte := []byte(head)
			res = append(res, head_byte...)
			res = append(res, file_data...)

			return res
		}
	}

	res = []byte(HTTP_NOTFOUND + HTTP_SPLIT + HTTP_SPLIT + "[event_static:65]: Can't find this file")

	return res
}
