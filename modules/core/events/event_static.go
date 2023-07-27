package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"strconv"
	"strings"
	"time"
)

const (
	HTTP_DEFAULT_CONTENT_TYPE = "text/html"
)

// handle static events
// if requests want to keep-alive, we use write bytes,
// if Content-Type is close, we write bytes and close this connection
// Recursion "Handle_event" isn't a problem, because it
// will pause when TCP buffer is None.
func Static_event(d listener.ListenData, path string, ev Event) {
	if ev.Req_.Connection == "keep-alive" {
		res := get_res_bytes(d, path, ev.Req_.Connection)
		write_bytes(ev, res)
		Handle_event(ev) // recursion
	} else {
		res := get_res_bytes(d, path, ev.Req_.Connection)
		write_bytes_close(ev, res)
	}
}

func get_res_bytes(lisdata listener.ListenData, path string, connection string) []byte {
	// if config.GOs == "windows" {
	// 	path = "/" + path
	// }
	var file_data = cache.Get_data_from_cache(path)

	res := response.Response_init() // Create a res Object
	res.Set_first_line(200, "OK")
	res.Set_header("Server", "Fast-Https")
	res.Set_header("Date", time.Now().String())

	if file_data != nil { // Not Fount

		res.Set_header("Content-Type", get_content_type(path))
		res.Set_header("Content-Length", strconv.Itoa(len(file_data)))
		if lisdata.Zip == 1 {
			res.Set_header("Content-Encoding", "gzip")
		}
		res.Set_header("Connection", connection)

		res.Set_body(file_data)
		return res.Generate_response()
	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + item
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			res.Set_header("Content-Type", get_content_type(path))
			res.Set_header("Content-Length", strconv.Itoa(len(file_data)))
			if lisdata.Zip == 1 {
				res.Set_header("Content-Encoding", "gzip")
			}
			res.Set_header("Connection", connection)

			res.Set_body(file_data)
			return res.Generate_response()
		}
	}

	return response.Default_not_found
}

// get this endpoint's content type
// user can define mime.types in confgure
func get_content_type(path string) string {
	path_type := strings.Split(path, ".")

	if path_type == nil {
		return HTTP_DEFAULT_CONTENT_TYPE
	}
	pointAfter := path_type[len(path_type)-1]
	row := config.GContentTypeMap[pointAfter]
	if row == "" {
		sep := "?"
		index := strings.Index(pointAfter, sep)
		if index != -1 { // if "?" exists
			pointAfter = pointAfter[:index] // delete chars from "?"
		}
		secondFind := config.GContentTypeMap[pointAfter]
		if secondFind != "" {
			return secondFind
		} else {
			return HTTP_DEFAULT_CONTENT_TYPE
		}
	}
	return row
}
