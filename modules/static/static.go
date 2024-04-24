package static

import (
	"bytes"
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"io"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	HTTP_DEFAULT_CONTENT_TYPE = "application/octet-stream"
)

func init() {
	core.RRHandlerRegister(config.LOCAL, HandelSlash, StaticEvent, nil)
}

// don't forget closing file.
func fileFdSize(pathName string) (file *os.File, size int64) {
	var err error
	file, err = os.Open(pathName)
	if err != nil {
		message.PrintErr("no such file")
		return nil, -1
	}

	var info fs.FileInfo
	info, err = file.Stat()
	if err != nil {
		return nil, -1
	}
	size = info.Size()

	return
}

func getResBytes(lisdata *listener.ListenCfg,
	path string, connection string, ev *core.Event) int {
	// if config.GOs == "windows" {
	// 	path = "/" + path
	// }
	// Handle request like this :
	// Simple-Line-Icons4c82.ttf?-i3a2kk
	path_type := strings.Split(path, "?")
	path = path_type[0]
	// var file_data = cache.Get_data_from_cache(path)
	// var file_data = []byte("cache.Get_data_from_cache(path)")
	file, file_size := fileFdSize(path)

	ev.RR.Res_.SetFirstLine(200, "OK")
	ev.RR.Res_.SetHeader("Server", "Fast-Https")
	ev.RR.Res_.SetHeader("Date", time.Now().String())

	if file != nil {

		ev.RR.Res_.SetHeader("Content-Type", getContentType(path))
		ev.RR.Res_.SetHeader("Content-Length", strconv.Itoa(int(file_size)))
		if lisdata.Zip == 1 {
			ev.RR.Res_.SetHeader("Content-Encoding", "gzip")
		}
		ev.RR.Res_.SetHeader("Connection", connection)

		// write first line and headers
		ev.Conn.Write(ev.RR.Res_.GenerateHeaderBytes())

		for {
			// 读取文件内容
			n, err := file.Read(ev.RR.ReqBuf)
			if err != nil {
				if err != io.EOF {
					file.Close()
					return -10
				}
				break
			}

			// 发送读取到的内容
			_, err = ev.Conn.Write(ev.RR.ReqBuf[:n])
			if err != nil {
				return -10
			}
		}

		core.LogOther(&ev.Log, "status", "200")
		core.LogOther(&ev.Log, "size", strconv.Itoa(int(file_size)))

		file.Close()
		return 1 // find source
	} // Not Found

	for _, item := range lisdata.StaticIndex { // Find files in default Index array

		realPath := path + item
		file, file_size := fileFdSize(realPath)

		if file != nil {

			ev.RR.Res_.SetHeader("Content-Type", getContentType(realPath))
			ev.RR.Res_.SetHeader("Content-Length", strconv.Itoa(int(file_size)))
			if lisdata.Zip == 1 {
				ev.RR.Res_.SetHeader("Content-Encoding", "gzip")
			}
			ev.RR.Res_.SetHeader("Connection", connection)

			for {
				// 读取文件内容
				n, err := file.Read(ev.RR.ReqBuf)
				if err != nil {
					if err != io.EOF {
						file.Close()
						return -10
					}
					break
				}

				// 发送读取到的内容
				_, err = ev.Conn.Write(ev.RR.ReqBuf[:n])
				if err != nil {
					return -10
				}
			}

			core.LogOther(&ev.Log, "status", "200")
			core.LogOther(&ev.Log, "size", strconv.Itoa(int(file_size)))
			file.Close()
			return 1 // find source
		}
	}

	core.LogOther(&ev.Log, "status", "404")
	core.LogOther(&ev.Log, "size", "50")
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

/*
 *************************************
 ****** Interfaces are as follows ****
 *************************************
 */

func HandelSlash(cfg *listener.ListenCfg, ev *core.Event) bool {
	if ev.RR.OriginPath == "" && cfg.Path != "/" {
		event301(ev, ev.RR.Req_.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return false
	}
	return true
}

// handle static events
// if requests want to keep-alive, we use write bytes,
// if Content-Type is close, we write bytes and close this connection
// Recursion "Handle_event" isn't a problem, because it
// will pause when TCP buffer is None.
func StaticEvent(cfg *listener.ListenCfg, ev *core.Event) {

	path := ev.RR.OriginPath
	if cfg.Path != "/" {
		path = cfg.StaticRoot + path
	} else {
		path = cfg.StaticRoot + ev.RR.Req_.Path
	}
	// ev.WriteResponse(ev.RR.Res_.GenerateResponse())

	if ev.RR.Req_.IsKeepalive() {
		res := getResBytes(cfg, path, ev.RR.Req_.GetConnection(), ev)
		if res == -1 {
			ev.RR.Res_ = response.DefaultNotFound()
		}
		ev.Reuse = true
		// HandleEvent(ev) // recursion
	} else {
		res := getResBytes(cfg, path, ev.RR.Req_.GetConnection(), ev)
		if res == -1 {
			ev.RR.Res_ = response.DefaultNotFound()
		}
		ev.Close()
	}
	core.Log(&ev.Log, ev, "")
	core.LogClear(&ev.Log)

	//fmt.Println(runtime.NumGoroutine(), GetGID())
}

func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
