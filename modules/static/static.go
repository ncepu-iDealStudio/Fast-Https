package static

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

func init() {
	core.RRHandlerRegister(config.LOCAL, HandelSlash, StaticEvent)
}

func HandelSlash(cfg listener.ListenCfg, ev *core.Event) bool {
	if ev.RR.OriginPath == "" && cfg.Path != "/" {
		event_301(ev, ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return false
	}
	return true
}

// handle static events
// if requests want to keep-alive, we use write bytes,
// if Content-Type is close, we write bytes and close this connection
// Recursion "Handle_event" isn't a problem, because it
// will pause when TCP buffer is None.
func StaticEvent(cfg listener.ListenCfg, ev *core.Event) {

	path := ev.RR.OriginPath
	if cfg.Path != "/" {
		path = cfg.StaticRoot + path
	} else {
		path = cfg.StaticRoot + ev.RR.Req_.Path
	}

	if ev.RR.Req_.IsKeepalive() {
		res := getResBytes(cfg, path, ev.RR.Req_.GetHeader("Connection"), ev)
		if res == -1 {
			ev.WriteData(response.DefaultNotFound())
		} else {
			ev.WriteData(ev.RR.Res_.GenerateResponse())
		}

		message.PrintAccess(ev.Conn.RemoteAddr().String(),
			"STATIC Event"+ev.Log, "\""+ev.RR.Req_.Headers["User-Agent"]+"\"")

		ev.Log_clear()

		ev.Reuse = true
		// HandleEvent(ev) // recursion
	} else {
		res := getResBytes(cfg, path, ev.RR.Req_.GetHeader("Connection"), ev)
		if res == -1 {
			ev.WriteDataClose(response.DefaultNotFound())
		} else {
			ev.WriteDataClose(ev.RR.Res_.GenerateResponse())
		}

		message.PrintAccess(ev.Conn.RemoteAddr().String(), "STATIC Event"+ev.Log,
			"\""+ev.RR.Req_.Headers["User-Agent"]+"\"")
		ev.Log_clear()
	}
}

func getResBytes(lisdata listener.ListenCfg,
	path string, connection string, ev *core.Event) int {
	// if config.GOs == "windows" {
	// 	path = "/" + path
	// }
	var file_data = cache.Get_data_from_cache(path)

	ev.RR.Res_.SetFirstLine(200, "OK")
	ev.RR.Res_.SetHeader("Server", "Fast-Https")
	ev.RR.Res_.SetHeader("Date", time.Now().String())

	if file_data != nil { // Not Fount

		ev.RR.Res_.SetHeader("Content-Type", getContentType(path))
		ev.RR.Res_.SetHeader("Content-Length", strconv.Itoa(len(file_data)))
		if lisdata.Zip == 1 {
			ev.RR.Res_.SetHeader("Content-Encoding", "gzip")
		}
		ev.RR.Res_.SetHeader("Connection", connection)

		ev.RR.Res_.SetBody(file_data)

		ev.Log += " 200 " + strconv.Itoa(len(file_data))

		return 1 // find source
	}

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + item
		file_data = cache.Get_data_from_cache(realPath)

		if file_data != nil {

			ev.RR.Res_.SetHeader("Content-Type", getContentType(path))
			ev.RR.Res_.SetHeader("Content-Length", strconv.Itoa(len(file_data)))
			if lisdata.Zip == 1 {
				ev.RR.Res_.SetHeader("Content-Encoding", "gzip")
			}
			ev.RR.Res_.SetHeader("Connection", connection)

			ev.RR.Res_.SetBody(file_data)
			ev.Log += " 200 " + strconv.Itoa(len(file_data))

			return 1 // find source
		}
	}

	ev.Log += " 404 50"
	return -1
}

// get this endpoint's content type
// user can define mime.types in confgure
func getContentType(path string) string {
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
			// delete chars from "?" to the end of string
			pointAfter = pointAfter[:index]
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