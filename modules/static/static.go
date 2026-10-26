// Package static 提供本地文件、目录索引与 gzip 压缩的静态资源服务。
package static

import (
	"fast-https/config"
	"fast-https/modules/compress"
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
	"io"
	"io/fs"
	"net/http"
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
func fileFdSize(pathName string) (file *os.File, size int64, modtime time.Time) {
	var err error
	file, err = os.Open(pathName)
	if err != nil {
		logging.PrintWarn("no such file")
		return nil, -1, time.Time{}
	}

	var info fs.FileInfo
	info, err = file.Stat()
	if err != nil {
		file.Close()
		return nil, -1, time.Time{}
	}
	size = info.Size()
	if info.IsDir() { // pathName is a dir
		file.Close()
		return nil, -1, time.Time{}
	}
	return file, size, info.ModTime()
}

func writeHeader(rr *core.RRcircle, firstLineCode int, reason string) {
	rr.Res.SetFirstLine(firstLineCode, reason)
	rr.Res.SetHeader("Server", "Fast-Https")
	rr.Res.SetHeader("Date", response.CachedDate())
}

// fileReadWrite 把文件流式写给客户端。
// io.Copy 经 net.TCPConn.ReadFrom 自动走 sendfile 零拷贝（Linux），
// Windows 上回退普通拷贝。
func fileReadWrite(file *os.File, ev *core.Event) int {
	if _, err := io.Copy(ev.Conn, file); err != nil {
		file.Close()
		return -10
	}
	file.Close()
	return 0
}

func dofind(path string, try *listener.Try, cfg *listener.ListenCfg) (found bool, file *os.File, file_size int64, modtime time.Time) {
	file, file_size, modtime = fileFdSize(path)

	if file == nil { // Not Found
		for _, item := range try.Files { // Find files in default Index array
			// Join 而不是裸拼接：realPath 经 Clean 后目录请求不再带
			// 结尾的 "/"，靠请求路径的结尾斜杠拼索引文件会拼出
			// "rootindex.html" 这样的路径
			p := filepath.Join(path, item)
			file, file_size, modtime = fileFdSize(p)
			if file != nil {
				found = true
				break
			}
		}
	} else {
		found = true
	}

	if !found && try.Next != "" {
		file, file_size, modtime = fileFdSize(cfg.StaticRoot + "/" + try.Next)
		if file != nil {
			found = true
		}
	}

	return found, file, file_size, modtime
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
	var modtime time.Time
	var found bool

	for _, try := range lisdata.Trys {
		if mathed := try.UriRe.Match([]byte(rr.Req.Path)); !mathed && try.Next == "" {
			continue
		}
		found, file, file_size, modtime = dofind(realPath, &try, lisdata)
		if found || try.Next != "" {
			break
		}
	}

	if lisdata.Trys == nil {
		found, file, file_size, modtime = dofind(realPath, &listener.Try{Files: lisdata.StaticIndex}, nil)
	}

	if !found {
		log.Status(404)
		log.Size(50)
		return -1
	}

	name := file.Name()
	lastModified := modtime.UTC().Format(http.TimeFormat)

	// 304/416 无正文：h2 交给协议层统一写出，h1 直接写头返回
	writeNoBody := func(code int, reason string) int {
		if rr.Req != nil && rr.Req.H2 {
			log.Status(code)
			log.Size(0)
			return 1
		}
		if _, err := ev.Conn.Write(ev.RR.Res.GenerateHeaderBytes()); err != nil {
			return -10
		}
		log.Status(code)
		log.Size(0)
		return 1
	}

	// 条件请求：If-Modified-Since 不早于文件修改时间 → 304
	if ims := rr.Req.HeaderFold("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil && !modtime.Truncate(time.Second).After(t) {
			file.Close()
			writeHeader(&rr, 304, "Not Modified")
			rr.Res.SetHeader("Last-Modified", lastModified)
			rr.Res.SetHeader("Connection", connection)
			return writeNoBody(304, "Not Modified")
		}
	}

	// Range 只对 GET 生效；存在 Range 时跳过压缩协商（单段范围）。
	rngStart, rngEnd := int64(-1), int64(-1)
	if rr.Req != nil && rr.Req.Method == "GET" {
		if spec := rr.Req.HeaderFold("Range"); spec != "" {
			s, e, st := parseRange(spec, file_size)
			if st < 0 {
				// 416：范围无法满足
				file.Close()
				writeHeader(&rr, 416, "Range Not Satisfiable")
				rr.Res.SetHeader("Content-Range", "bytes */"+strconv.FormatInt(file_size, 10))
				rr.Res.SetHeader("Content-Length", "0")
				rr.Res.SetHeader("Connection", connection)
				return writeNoBody(416, "Range Not Satisfiable")
			}
			if st > 0 {
				rngStart, rngEnd = s, e
			}
		}
	}

	var payload []byte
	encoding := ""
	stream := true
	if rngStart == -1 {
		encoding = encodingWanted(lisdata.Zip, acceptEncoding(ev))
		if encoding != "" {
			raw, err := io.ReadAll(file)
			file.Close()
			file = nil
			if err != nil {
				return -10
			}
			compressed, err := compress.CompressBytes(raw, encoding)
			if err != nil {
				return -10
			}
			payload = compressed
			stream = false
		}
	}
	payloadLen := file_size
	if rngStart >= 0 {
		payloadLen = rngEnd - rngStart + 1
	}

	if rngStart >= 0 {
		writeHeader(&rr, 206, "Partial Content")
		rr.Res.SetHeader("Content-Range", "bytes "+
			strconv.FormatInt(rngStart, 10)+"-"+strconv.FormatInt(rngEnd, 10)+"/"+
			strconv.FormatInt(file_size, 10))
	} else {
		writeHeader(&rr, 200, "OK")
	}
	rr.Res.SetHeader("Content-Type", getContentType(name))
	rr.Res.SetHeader("Content-Length", strconv.Itoa(int(payloadLen)))
	rr.Res.SetHeader("Last-Modified", lastModified)
	if !stream {
		rr.Res.SetHeader("Content-Encoding", encoding)
	}
	if lisdata.Zip != config.ZIP_NONE {
		// 响应随 Accept-Encoding 变化，告知下游缓存不要把压缩结果
		// 串给不支持该编码的客户端
		rr.Res.SetHeader("Vary", "Accept-Encoding")
	}
	rr.Res.SetHeader("Connection", connection)

	// HTTP/2 由协议层负责分帧与流控，不能直接写底层连接，
	// 把响应体交给 Res，由 EventWrite 统一写出。
	if rr.Req != nil && rr.Req.H2 {
		var body []byte
		switch {
		case rngStart >= 0:
			if _, err := file.Seek(rngStart, io.SeekStart); err != nil {
				file.Close()
				return -10
			}
			var err error
			body, err = io.ReadAll(io.LimitReader(file, rngEnd-rngStart+1))
			file.Close()
			if err != nil {
				return -10
			}
		case !stream:
			body = payload
		default:
			var err error
			body, err = io.ReadAll(file)
			file.Close()
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
		switch {
		case rngStart >= 0:
			if _, err := file.Seek(rngStart, io.SeekStart); err != nil {
				file.Close()
				return -10
			}
			if _, err := io.CopyN(ev.Conn, file, rngEnd-rngStart+1); err != nil {
				file.Close()
				return -10
			}
			file.Close()
		case !stream:
			if _, err := ev.Conn.Write(payload); err != nil {
				return -10
			}
		default:
			if fileReadWrite(file, ev) != 0 {
				return -10
			}
		}
	} else if file != nil {
		file.Close()
	}

	// log
	log.Status(200)
	log.Size(int(payloadLen))

	return 1 // find source
}

// parseRange 解析单段 "bytes=" 范围请求。
// 返回 (start, end, status)：status 1 = 有效范围；0 = 无可应用的
// 范围（多段范围或单位不是 bytes，按 200 全量返回）；-1 = 畸形或
// 越界范围，应回 416。
func parseRange(spec string, size int64) (int64, int64, int) {
	const prefix = "bytes="
	if !strings.HasPrefix(spec, prefix) {
		return 0, 0, 0
	}
	spec = spec[len(prefix):]
	if strings.Contains(spec, ",") { // 多段范围：忽略，全量返回
		return 0, 0, 0
	}
	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, -1
	}
	startStr, endStr := spec[:dash], spec[dash+1:]

	if startStr == "" { // 后缀范围：-N 取最后 N 字节
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, -1
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, 1
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, -1
	}
	end := size - 1
	if endStr != "" {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < start {
			return 0, 0, -1
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, 1
}

// directoryRedirect 判断请求指向一个真实存在的目录却没有结尾斜杠，
// 返回应当 301 过去的地址（query 原样带上）。
func directoryRedirect(root, reqPath, filePath string) (string, bool) {
	if reqPath == "" || strings.HasSuffix(reqPath, "/") {
		return "", false
	}
	pathOnly, query, _ := strings.Cut(reqPath, "?")
	// 末段带 "." 基本是文件请求（css/js/图片/html），跳过这次 Stat，
	// 不给每个静态资源都多叠一次系统调用。
	if last := pathOnly[strings.LastIndex(pathOnly, "/")+1:]; last == "" || strings.Contains(last, ".") {
		return "", false
	}
	fsPath, _, _ := strings.Cut(filePath, "?")

	fsPath, inRoot := staticPathInRoot(root, fsPath)
	if !inRoot {
		return "", false
	}
	info, err := os.Stat(fsPath)
	if err != nil || !info.IsDir() {
		return "", false
	}

	target := pathOnly + "/"
	if query != "" {
		target += "?" + query
	}
	return target, true
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

// encodingWanted 按配置与客户端 Accept-Encoding 协商响应压缩编码。
// ZIP_GZIP_BR 两种都配置时优先选压缩率更好的 br，客户端不认再回退
// gzip；q 值只判断是否接受（q>0），不做加权择优。
// 返回空串表示不压缩。
func encodingWanted(zip uint16, accept string) string {
	switch zip {
	case config.ZIP_GZIP:
		if request.HasToken(accept, "gzip") {
			return "gzip"
		}
	case config.ZIP_BR:
		if request.HasToken(accept, "br") {
			return "br"
		}
	case config.ZIP_GZIP_BR:
		if request.HasToken(accept, "br") {
			return "br"
		}
		if request.HasToken(accept, "gzip") {
			return "gzip"
		}
	}
	return ""
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
	// 应用防火墙检查已收敛到 events.EventHandler 统一执行
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

	// 目录请求缺结尾斜杠：先 301 补齐，再谈取文件。
	// nginx 的 try_files $uri/ 就是这个行为，缺了它 /about 会直接 404，
	// 页面里的相对链接也会因为基准路径不同而失效。
	if target, ok := directoryRedirect(cfg.StaticRoot, rr.Req.Path, path); ok {
		event301(ev, target)
		return
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
