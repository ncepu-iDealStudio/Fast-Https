package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"strconv"
	"strings"
)

// var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func StaticEvent(path string) []byte {

	// fmt.Println("load file, Path", path)
	var res []byte
	var file_data = cache.Get_data_from_cache(path)

	if file_data != nil { // Not Fount
		path_type := strings.Split(path, ".")

		head := "HTTP/1.1 200 OK\r\n"
		head += "Content-Type: " + config.G_ContentTypeMap[path_type[len(path_type)-1]] + "\r\n"
		head += "Content-Length: " + strconv.Itoa(len(file_data)) + "\r\n"
		head += "\r\n"

		head_byte := []byte(head)
		res = append(res, head_byte...)
		res = append(res, file_data...)

	} else {
		res = []byte("HTTP/1.1 404 \r\n\r\nNOTFOUNT")
	}

	return res
}
