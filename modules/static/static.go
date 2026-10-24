// Package static 提供本地文件、目录索引与 gzip 压缩的静态资源服务。
package static

import (
	"fast-https/config"
	"fast-https/modules/appfirewall"
	"fast-https/modules/compress"
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Register installs the static file handler for location type local.
func Register() {
	core.RegisterHandler(config.LOCAL, HandelSlash, StaticEvent, nil)
}

// don't forget closing file.
func fileFdSize(pathName string) (file *os.File, size int64) {
	var err error
	file, err = os.Open(pathName)
	if err != nil {
		logging.PrintWarn("no such file")
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
		if n == 0 || err != nil {
			file.Close()
			if err != nil && err != io.EOF {
				return -10
			}
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

func getResBytes(log *dynlog.DynLogger, lisdata *listener.ListenCfg,
	path string, connection string, ev *core.Event) int {

	rr := ev.RR
	// Handle request like this :
	// Simple-Line-Icons4c82.ttf?-i3a2kk
	path_type := strings.Split(path, "?")

	realPath := path_type[0]
	// /../ 之类的逃逸路径按未找到处理，不读任何文件
	realPath, inRoot := staticPathInRoot(lisdata.StaticRoot, realPath)
	if !inRoot {
		log.Status(404)
		log.Size(50)
		return -1
	}
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
		log.Status(404)
		log.Size(50)
		return -1
	}

	name := file.Name()
	payloadLen := file_size
	var payload []byte
	stream := true
	if gzipWanted(lisdata.Zip, acceptEncoding(ev)) {
		raw, err := io.ReadAll(file)
		file.Close()
		file = nil
		if err != nil {
			return -10
		}
		compressed, err := compress.CompressBytesGzip(raw)
		if err != nil {
			return -10
		}
		payload = compressed
		payloadLen = int64(len(payload))
		stream = false
	}

	writeHeader(&rr, 200)
	rr.Res.SetHeader("Content-Type", getContentType(name))
	rr.Res.SetHeader("Content-Length", strconv.Itoa(int(payloadLen)))
	if !stream {
		rr.Res.SetHeader("Content-Encoding", "gzip")
	}
	rr.Res.SetHeader("Connection", connection)

	// HTTP/2 由协议层负责分帧与流控，不能直接写底层连接，
	// 把响应体交给 Res，由 EventWrite 统一写出。
	if rr.Req != nil && rr.Req.H2 {
		var body []byte
		if !stream {
			body = payload
			if file != nil {
				file.Close()
				file = nil
			}
		} else {
			var err error
			body, err = io.ReadAll(file)
			if file != nil {
				file.Close()
				file = nil
			}
			if err != nil {
				return -10
			}
		}
		rr.Res.Body = body
		log.Status(200)
		log.Size(len(body))
		return 1
	}

	if _, err := ev.Conn.Write(ev.RR.Res.GenerateHeaderBytes()); err != nil {
		if file != nil {
			file.Close()
		}
		return -10
	}

	if ev.RR.Req == nil || ev.RR.Req.Method != "HEAD" {
		if !stream {
			if _, err := ev.Conn.Write(payload); err != nil {
				return -10
			}
		} else if fileReadWrite(file, ev) != 0 {
			return -10
		}
	} else if file != nil {
		file.Close()
	}

	// log
	log.Status(200)
	log.Size(int(payloadLen))

	return 1 // find source
}

// staticPathInRoot 清洗拼接后的文件路径并校验没有逃出站点根目录。
// StaticRoot 在配置加载时已解析为绝对路径；rooted 是 StaticRoot 与
// URL 路径直接拼接的结果。本服务不做 URL 解码，%2e%2e 不会被还原
// 成 ..。返回清洗后的路径；ok=false 表示逃逸，调用方按 404 处理。
func staticPathInRoot(root, rooted string) (string, bool) {
	rootAbs := filepath.Clean(root)
	full := filepath.Clean(rooted)
	if full != rootAbs && !strings.HasPrefix(full, rootAbs+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// get this endpoint's content type
// user can define mime.types in confgure
func getContentType(path string) string {
	path_type := strings.Split(path, ".")

	if path_type == nil {
		return config.HTTP_DEFAULT_CONTENT_TYPE
	}
	pointAfter := path_type[len(path_type)-1]
	row := config.MIME()[pointAfter]
	if row == "" {
		sep := "?"
		index := strings.Index(pointAfter, sep)
		if index != -1 { // if "?" exists
			// delete chars from "?" to the end of string
			pointAfter = pointAfter[:index]
		}
		secondFind := config.MIME()[pointAfter]
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

func gzipWanted(zip uint16, accept string) bool {
	if zip != config.ZIP_GZIP && zip != config.ZIP_GZIP_BR {
		return false
	}
	return request.HasToken(accept, "gzip")
}

func acceptEncoding(ev *core.Event) string {
	if ev == nil || ev.RR.Req == nil {
		return ""
	}
	return ev.RR.Req.HeaderFold("Accept-Encoding")
}

func NotFoundResponse(cfg *listener.ListenCfg) *response.Response {
	res := response.DefaultNotFound()
	if cfg != nil {
		response.UseFile(res, cfg.ErrorNotFound)
	}
	return res
}

func HandelSlash(cfg *listener.ListenCfg, ev *core.Event) bool {
	if ev.RR.OriginPath == "" && cfg.Path != "/" {
		event301(ev, ev.RR.Req.Path[ev.RR.PathLocation[0]:ev.RR.PathLocation[1]]+"/")
		return false
	}
	if !appfirewall.HandleAppFireWall(cfg, ev.RR.Req) {
		ev.RR.Res = response.DefaultForbidden()
		ev.WriteResponseClose(nil)
		return false
	}
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

	log := dynlog.DynLogger{}

	if rr.Req.IsKeepalive() {
		res := getResBytes(&log, cfg, path, rr.Req.GetConnection(), ev)
		if res == -1 {
			ev.RR.Res = NotFoundResponse(cfg)
			_ = ev.WriteResponse(nil)
		}
		ev.Reuse = true
	} else {
		res := getResBytes(&log, cfg, path, rr.Req.GetConnection(), ev)
		if res == -1 {
			ev.RR.Res = NotFoundResponse(cfg)
			_ = ev.WriteResponse(nil)
		}
		if !rr.Req.H2 {
			ev.Close()
		}
	}

	dynlog.Log(&log, ev)
}
