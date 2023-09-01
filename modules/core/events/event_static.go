package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
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
func Static_event(cfg listener.ListenCfg, path string, ev *Event) {

	if cfg.Path != "/" {
		path = cfg.StaticRoot + path
	} else {
		path = cfg.StaticRoot + ev.Req_.Path
	}

	if ev.Req_.Is_keepalive() {
		res := get_res_bytes(cfg, path, ev.Req_.Get_header("Connection"), ev)
		write_bytes(ev, res)
		message.PrintAccess(ev.Conn.RemoteAddr().String(), " STATIC Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])
		Handle_event(ev) // recursion
	} else {
		res := get_res_bytes(cfg, path, ev.Req_.Get_header("Connection"), ev)
		message.PrintAccess(ev.Conn.RemoteAddr().String(), " STATIC Events "+ev.Log, " "+ev.Req_.Headers["User-Agent"])
		write_bytes_close(ev, res)
	}
}

func get_res_bytes(lisdata listener.ListenCfg, path string, connection string, ev *Event) []byte {
	// if config.GOs == "windows" {
	// 	path = "/" + path
	// }
	var file_data = cache.Get_data_from_cache(path)

	ev.Res_ = response.Response_init() // Create a res Object
	ev.Res_.Set_first_line(200, "OK")
	ev.Res_.Set_header("Server", "Fast-Https")
	ev.Res_.Set_header("Date", time.Now().String())

	if file_data != nil { // Not Fount

		ev.Res_.Set_header("Content-Type", get_content_type(path))
		ev.Res_.Set_header("Content-Length", strconv.Itoa(len(file_data)))
		if lisdata.Zip == 1 {
			ev.Res_.Set_header("Content-Encoding", "gzip")
		}
		ev.Res_.Set_header("Connection", connection)

		ev.Res_.Set_body(file_data)

		ev.Log += " 200 " + strconv.Itoa(len(file_data))
		return ev.Res_.Generate_response() // find source
	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + item
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			ev.Res_.Set_header("Content-Type", get_content_type(path))
			ev.Res_.Set_header("Content-Length", strconv.Itoa(len(file_data)))
			if lisdata.Zip == 1 {
				ev.Res_.Set_header("Content-Encoding", "gzip")
			}
			ev.Res_.Set_header("Connection", connection)

			ev.Res_.Set_body(file_data)
			ev.Log += " 200 " + strconv.Itoa(len(file_data))

			return ev.Res_.Generate_response() // find source
		}
	}

	ev.Log += " 404 50"
	return response.Default_not_found() // not found source
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
