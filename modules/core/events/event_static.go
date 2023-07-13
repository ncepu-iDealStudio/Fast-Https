package events

import (
	"fast-https/modules/cache"
)

// var data = "HTTP/1.1 200 OK\r\nConnection: keep-alive\r\n\r\nHello World"

func StaticEvent(path string) []byte {

	// fmt.Println("load file, Path", path)

	var data = cache.Get_data_from_cache(path)

	return data
}
