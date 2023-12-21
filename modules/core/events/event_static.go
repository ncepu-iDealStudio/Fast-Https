package events

import (
	"fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core"
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
func Static_event(cfg listener.ListenCfg, ev *core.Event) {

	path := ev.RR.OriginPath
	if cfg.Path != "/" {
		path = cfg.StaticRoot + path
	} else {
		path = cfg.StaticRoot + ev.RR.Req_.Path
	}

	if ev.RR.Req_.Is_keepalive() {
		res := get_res_bytes(cfg, path, ev.RR.Req_.Get_header("Connection"), ev)
		if res == -1 {
			ev.Write_bytes(response.Default_not_found())
		} else {
			ev.Write_bytes(ev.RR.Res_.Generate_response())
		}

		message.PrintAccess(ev.Conn.RemoteAddr().String(), "STATIC Event"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
		ev.Log = ""
		Handle_event(ev) // recursion
	} else {
		res := get_res_bytes(cfg, path, ev.RR.Req_.Get_header("Connection"), ev)
		if res == -1 {
			ev.Write_bytes(response.Default_not_found())
		} else {
			ev.Write_bytes(ev.RR.Res_.Generate_response())
		}
		message.PrintAccess(ev.Conn.RemoteAddr().String(), "STATIC Event"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
		ev.Log = ""
	}
}

func get_res_bytes(lisdata listener.ListenCfg, path string, connection string, ev *core.Event) int {
	// if config.GOs == "windows" {
	// 	path = "/" + path
	// }
	var file_data = cache.Get_data_from_cache(path)

	ev.RR.Res_ = response.Response_init() // Create a res Object
	ev.RR.Res_.Set_first_line(200, "OK")
	ev.RR.Res_.Set_header("Server", "Fast-Https")
	ev.RR.Res_.Set_header("Date", time.Now().String())

	if file_data != nil { // Not Fount

		ev.RR.Res_.Set_header("Content-Type", get_content_type(path))
		ev.RR.Res_.Set_header("Content-Length", strconv.Itoa(len(file_data)))
		if lisdata.Zip == 1 {
			ev.RR.Res_.Set_header("Content-Encoding", "gzip")
		}
		ev.RR.Res_.Set_header("Connection", connection)

		ev.RR.Res_.Set_body(file_data)

		ev.Log += " 200 " + strconv.Itoa(len(file_data))

		return 1 // find source
	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + item
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			ev.RR.Res_.Set_header("Content-Type", get_content_type(path))
			ev.RR.Res_.Set_header("Content-Length", strconv.Itoa(len(file_data)))
			if lisdata.Zip == 1 {
				ev.RR.Res_.Set_header("Content-Encoding", "gzip")
			}
			ev.RR.Res_.Set_header("Connection", connection)

			ev.RR.Res_.Set_body(file_data)
			ev.Log += " 200 " + strconv.Itoa(len(file_data))

			return 1 // find source
		}
	}

	ev.Log += " 404 50"
	return -1
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
			pointAfter = pointAfter[:index] // delete chars from "?" to the end of string
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
