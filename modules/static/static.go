package static

import (
	"bytes"
	"fast-https/config"
	"fast-https/modules/appfirewall"
	"fast-https/modules/core"
	"fast-https/modules/core/events"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func init() {
	core.RRHandlerRegister(config.LOCAL, HandelSlash, StaticEvent, nil)
}

// don't forget closing file.
func fileFdSize(pathName string) (file *os.File, size int64) {
	var err error
	file, err = os.Open(pathName)
	if err != nil {
		message.PrintWarn("no such file")
		return nil, -1
	}

	var info fs.FileInfo
	info, err = file.Stat()
	if err != nil {
		return nil, -1
	}
	size = info.Size()
	if info.IsDir() { // pathName is a dir
		return nil, -1
	}
	return
}

func writeHeader(rr *core.RRcircle, firstLineCode int) {
	rr.Res.SetFirstLine(firstLineCode, "OK")
	rr.Res.SetHeader("Server", "Fast-Https")
	rr.Res.SetHeader("Date", time.Now().String())
}

func fileReadWrite(file *os.File, ev *core.Event) int {
	rr := ev.RR
	for {
		// 读取文件内容
		n, err := file.Read(rr.ReqBuf)
		if err != nil {
			if err != io.EOF {
				file.Close()
				return -10
			}
			file.Close()
			break
		}

		// 发送读取到的内容
		_, err = ev.Conn.Write(rr.ReqBuf[:n])
		if err != nil {
			return -10
		}
	}
	return 0
}

func dofind(path string, try *listener.Try, cfg *listener.ListenCfg) (found bool, file *os.File, file_size int64) {
	file, file_size = fileFdSize(path)

	if file == nil { // Not Found
		for _, item := range try.Files { // Find files in default Index array
			p := path + item
			file, file_size = fileFdSize(p)
			if file != nil {
				found = true
				break
			}
		}
	} else {
		found = true
	}

	if !found && try.Next != "" {
		file, file_size = fileFdSize(cfg.StaticRoot + "/" + try.Next)
		if file != nil {
			found = true
		}
	}

	return found, file, file_size
}

func getResBytes(lisdata *listener.ListenCfg,
	path string, connection string, ev *core.Event) int {

	rr := ev.RR
	// Handle request like this :
	// Simple-Line-Icons4c82.ttf?-i3a2kk
	path_type := strings.Split(path, "?")

	realPath := path_type[0]
	// var file_data = cache.Get_data_from_cache(realPath)
	// var file_data = []byte("cache.Get_data_from_cache(realPath)")

	var file *os.File   // file fd
	var file_size int64 // file size
	var found bool

	for _, try := range lisdata.Trys {
		if mathed := try.UriRe.Match([]byte(rr.Req.Path)); !mathed && try.Next == "" {
			continue
		}
		found, file, file_size = dofind(realPath, &try, lisdata)
		if found || try.Next != "" {
			break
		}
	}

	if lisdata.Trys == nil {
		found, file, file_size = dofind(realPath, &listener.Try{Files: lisdata.StaticIndex}, nil)
	}

	if !found {
		core.LogOther(&ev.Log, "status", "404")
		core.LogOther(&ev.Log, "size", "50")
		return -1
	}

	writeHeader(&rr, 200)
	rr.Res.SetHeader("Content-Type", getContentType(file.Name()))
	rr.Res.SetHeader("Content-Length", strconv.Itoa(int(file_size)))
	if lisdata.Zip == 1 {
		rr.Res.SetHeader("Content-Encoding", "gzip")
	}
	rr.Res.SetHeader("Connection", connection)

	/*
		// h2 的开发过程要注释掉
		// 写头
		// write first line and headers
		ev.Conn.Write(ev.RR.Res.GenerateHeaderBytes())

		// 写body
		if fileReadWrite(file, ev) != 0 { // some error
			return -10
		}
	*/
	/*  ============ 以下 h2 逻辑 ============= */
	// 写头
	responseHeader := http.Header{}
	firstLine := strings.Split(ev.RR.Res.FirstLine, " ")
	if len(firstLine) != 3 {
		fmt.Println("-----------ev.RR.Res_.FirstLine-------------")
	}
	responseHeader.Add(":status", firstLine[1])
	for header, content := range ev.RR.Res.Headers {
		if header == "Connection" || header == "Content-Length" {
			continue
		}
		responseHeader.Add(header, content)
	}
	// fmt.Println(responseHeader)
	events.WriteHeader(ev, responseHeader)

	// // 写body
	var file_data = make([]byte, file_size)
	file.Read(file_data)
	events.H2EventWrite(ev, file_data)
	/* =========== h2 逻辑结束 ==================== */

	// log
	core.LogOther(&ev.Log, "status", "200")
	core.LogOther(&ev.Log, "size", strconv.Itoa(int(file_size)))
	core.LogClear(&ev.Log)

	return 1 // find source
}

// get this endpoint's content type
// user can define mime.types in confgure
func getContentType(path string) string {
	path_type := strings.Split(path, ".")

	if path_type == nil {
		return config.HTTP_DEFAULT_CONTENT_TYPE
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
			return config.HTTP_DEFAULT_CONTENT_TYPE
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
		event301(ev, ev.RR.Req.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return false
	}
	appfirewall.HandleAppFireWall(cfg, ev.RR.Req)
	return true
}

// handle static events
// if requests want to keep-alive, we use write bytes,
// if Content-Type is close, we write bytes and close this connection
func StaticEvent(cfg *listener.ListenCfg, ev *core.Event) {
	rr := ev.RR
	path := rr.OriginPath
	if cfg.Path != "/" {
		path = cfg.StaticRoot + path
	} else {
		path = cfg.StaticRoot + rr.Req.Path
	}

	if rr.Req.IsKeepalive() {
		res := getResBytes(cfg, path, rr.Req.GetConnection(), ev)
		if res == -1 {
			rr.Res = response.DefaultNotFound()
			// h2 dev need remove
			ev.Conn.Write(rr.Res.GenerateResponse())
		}
		ev.Reuse = true
	} else {
		res := getResBytes(cfg, path, rr.Req.GetConnection(), ev)
		if res == -1 {
			rr.Res = response.DefaultNotFound()
			// h2 dev need remove
			ev.Conn.Write(rr.Res.GenerateResponse())
		}
		if !rr.Req.H2 {
			ev.Close()
		}
	}
	core.Log(&ev.Log, ev, "")
	core.LogClear(&ev.Log)
}

func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}
