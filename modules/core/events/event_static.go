package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"log"
	"strconv"
	"strings"
)

// var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func StaticEvent(lisdata listener.ListenData, path string) []byte {

	var res []byte
	var file_data = cache.Get_data_from_cache(path)

	if file_data != nil { // Not Fount
		path_type := strings.Split(path, ".")

		log.Println("[Events]Get file: ", path)

		head := "HTTP/1.1 200 OK\r\n"
		head += "Content-Type: " + config.G_ContentTypeMap[path_type[len(path_type)-1]] + "\r\n"
		if lisdata.Gzip == 1 {
			head += "Content-Encoding: gzip" + "\r\n"
		}
		head += "Content-Length: " + strconv.Itoa(len(file_data)) + "\r\n"
		head += "\r\n"

		head_byte := []byte(head)
		res = append(res, head_byte...)
		res = append(res, file_data...)

	} else {
		log.Println("[Events]file not found: ", path)
		res = []byte("HTTP/1.1 404 \r\n\r\nNOTFOUNT")
	}

	return res
}
