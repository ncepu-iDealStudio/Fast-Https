package proxy

import (
	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"io"
	"net"
	"strconv"
	"strings"
)

const (
	TRY_READ_LEN        = 2048
	READ_BYTES_LEN      = 4096
	READ_BODY_BYTES_LEN = 1024
	CHUNCKED_BODY_SIZE  = 8192
	// interimResponse 表示读到 1xx 中间响应（100 Continue / 103 Early Hints 等），
	// 调用方应丢弃这段并继续读最终响应。101 是协议升级，不按中间响应处理。
	interimResponse = -5
)

type ReadKeepAlive struct {
	TryNum       int
	finalStr     []byte
	bodyPosition int
	res          *response.Response
	body         []byte
	p            *Proxy
	// emptyBody 表示刚解析的响应在协议上必然没有 body（HEAD / 204 / 304）。
	// 这类响应不需要等上游关闭，调用方可以复用连接。
	emptyBody bool
	// trailingBytes 表示空 body 响应的头部之后还读到了字节。上游违约时
	// 连接不能复用，否则残留字节会污染下一个请求。
	trailingBytes bool
}

/*
  - return -1 str is too short
    return -2 parse failed no such Header "Content-Length"
  - if parse successed, return a number that need to be read.
*/
func (rka *ReadKeepAlive) tryToParse(tmpData []byte) int {

	tmpLen := len(tmpData)
	if tmpLen < 4 {
		// str is too short
		return -1 // parse failed!
	}
	// fmt.Println(tmpData)

	i := strings.Index(string(tmpData), "\r\n\r\n")
	if i == -1 {
		// parse failed! "no \r\n\r\n"
		// caller need call this again maybe
		rka.TryNum = rka.TryNum + 1
		return -2
	}

	res := response.ResponseInit()
	res.HttpResParse(string(tmpData))
	rka.res = res
	rka.bodyPosition = i + 4

	// 1xx（101 升级除外）是中间响应：没有 body，后面还会跟最终响应。
	// 必须先于 HEAD 判断，否则会被当成最终响应结束读取并丢掉真正的响应。
	if code := responseStatusCode(res); code >= 100 && code < 200 && code != 101 {
		return interimResponse
	}

	// HEAD 的响应永远没有 body，即使头里带 Content-Length（那是 GET 的长度）。
	// 不先判 HEAD 会按 CL 去读一个永远不来的 body，直到读超时。
	if rka.isHeadRequest() {
		rka.body = nil
		rka.emptyBody = true
		rka.trailingBytes = tmpLen > i+4
		return 0
	}

	// 204 / 304 在协议上也没有 body。304 允许带 Content-Length（表示 200
	// 时正文的长度），不能按它去读正文，否则同样会等到读超时。
	if bodylessStatus(res) {
		rka.body = nil
		rka.emptyBody = true
		rka.trailingBytes = tmpLen > i+4
		return 0
	}

	var contentLength int
	if res.GetHeader("Content-Length") != "" {
		// fmt.Println(res.GetHeader("Content-Length"))
		contentLength, _ = strconv.Atoi(res.GetHeader("Content-Length"))
		NeedRead := contentLength - (tmpLen - i - 4)
		rka.body = tmpData[i+4:]
		return NeedRead
	} else if res.GetHeader("Transfer-Encoding") == "chunked" {
		rka.body = tmpData[i+4:]
		return -3
	} else {
		// HTTP/1.0 风格「靠关闭连接表示响应结束」，只能读到 EOF，
		// 且这条连接不进入复用池。
		rka.body = tmpData[i+4:]
		return -4
	}
}

// isHeadRequest 报告当前请求是否为 HEAD。rka.p / ev / Req 在单测里可能缺失。
func (rka *ReadKeepAlive) isHeadRequest() bool {
	if rka.p == nil || rka.p.ev == nil || rka.p.ev.RR.Req == nil {
		return false
	}
	return strings.EqualFold(rka.p.ev.RR.Req.Method, "HEAD")
}

// responseStatusCode 从状态行里取状态码；解析失败返回 0。
func responseStatusCode(res *response.Response) int {
	parts := strings.SplitN(res.FirstLine, " ", 3)
	if len(parts) < 2 {
		return 0
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return code
}

// bodylessStatus 报告状态码是否在协议上必然没有 body（204 / 304）。
func bodylessStatus(res *response.Response) bool {
	code := responseStatusCode(res)
	return code == 204 || code == 304
}

// readUntilEOF 读到上游关闭连接为止，把剩余 body 追加到响应里。
// 只有「既没有 Content-Length 也没有 chunked」的旧式响应会走到这里。
// 累计长度受 max_body_size 限制：无长度声明的响应不能让上游无限撑大内存。
func (rka *ReadKeepAlive) readUntilEOF() error {
	limit := maxChunkedBody()
	buf := make([]byte, READ_BODY_BYTES_LEN)
	for {
		n, err := rka.Read(buf)
		if n > 0 {
			rka.finalStr = append(rka.finalStr, buf[:n]...)
			rka.body = append(rka.body, buf[:n]...)
			if limit > 0 && len(rka.body) > limit {
				return ProxyErrorUpstreamBodyTooLarge
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				noteReadTimeout()
				return ProxyErrorReadFromUpstreamTimeout
			}
			return err
		}
		if n == 0 {
			return nil
		}
	}
}

// parseChunked 解码上游的 chunked 响应体，并按解码后的实际长度改写响应头。
// 此前实现只把带 chunk 帧的原始 body 原样透传，客户端会收到 chunk 元数据
// 以及与实际内容矛盾的 Content-Length。
func (rka *ReadKeepAlive) parseChunked() error {
	body, err := request.ReadChunked(rka.body, func() ([]byte, error) {
		buf := make([]byte, CHUNCKED_BODY_SIZE)
		n, err := rka.Read(buf)
		if err != nil {
			return nil, err
		}
		return buf[:n], nil
	}, maxChunkedBody())
	if err != nil {
		return err
	}
	rka.res.DelHeader("Transfer-Encoding")
	rka.res.SetHeader("Content-Length", strconv.Itoa(len(body)))
	rka.res.Body = body
	rka.body = body
	rka.finalStr = rka.res.GenerateResponse()
	return nil
}

func maxChunkedBody() int {
	if config.Current().Limit.MaxBodySize > 0 {
		return config.Current().Limit.MaxBodySize
	}
	return config.DEFAULT_MAX_BODY_SIZE
}

func (rka *ReadKeepAlive) Read(data []byte) (int, error) {
	if rka.p.ProxyType == config.PROXY_HTTP {
		return rka.p.ProxyConn.Read(data)
	} else if rka.p.ProxyType == config.PROXY_HTTPS {
		return rka.p.ProxyTlsConn.Read(data)
	} else {
		logging.PrintErr("--proxy read error")
		return 0, nil
	}
}

// debug by zhangjiayue
// ReadBytes 读满 Content-Length 承诺的 body。任何读错误都必须上抛：
// 上游半响应即断开/重置时，把截断数据当 200 回给客户端（更糟的是
// 落进缓存）等于投毒；超时同样按错误处理而不是交付半截 body。
func (rka *ReadKeepAlive) ReadBytes(size int) error {
	totalLen := size
	//ro.ProxyConn.SetReadDeadline(time.Now().Add(time.Second * 100))
	for totalLen > 0 {
		readLen := READ_BODY_BYTES_LEN
		if totalLen < READ_BODY_BYTES_LEN {
			readLen = totalLen
		}
		tmpBuf := make([]byte, readLen)
		tempLen, err := rka.Read(tmpBuf)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				noteReadTimeout()
				return ProxyErrorReadFromUpstreamTimeout
			}
			return err
		}
		rka.finalStr = append(rka.finalStr, tmpBuf[:tempLen]...)
		rka.body = append(rka.body, tmpBuf[:tempLen]...)
		totalLen -= tempLen
	}
	return nil
}

func (rka *ReadKeepAlive) proxyKeepAlive() error {
	tmpByte := make([]byte, TRY_READ_LEN)
	for {
		// 先解析已缓冲的数据：1xx 之后可能已经带着最终响应到达，
		// 这时不能再 Read，否则会一直等到超时。
		size := rka.tryToParse(rka.finalStr)
		switch {
		case size == 0: // 不需要再读
			return nil
		case size > 0:
			return rka.ReadBytes(size)
		case size == -3:
			if err := rka.parseChunked(); err != nil {
				logging.ConsoleLog.Debug().Msgf("proxy: decode chunked body: %v", err)
				return err
			}
			return nil
		case size == -4:
			// 无 Content-Length、无 chunked：按 HTTP/1.0 语义读到连接关闭。
			// 这条连接不会复用，调用方会据 ProxyNeedClose 关闭它。
			return rka.readUntilEOF()
		case size == interimResponse:
			// 丢弃中间响应，只保留同一段数据里最终响应的开头。
			rest := append([]byte(nil), rka.finalStr[rka.bodyPosition:]...)
			rka.finalStr = rest
			rka.body = nil
			rka.res = nil
			rka.emptyBody = false
			continue
		}
		// -1 / -2：头还没读完整，继续读。
		lenOnce, err := rka.Read(tmpByte)
		if err != nil {
			neterr, ok := err.(net.Error)
			if ok && neterr.Timeout() {
				noteReadTimeout()
				return ProxyErrorReadFromUpstreamTimeout
			} else if ok {
				logging.ConsoleLog.Debug().Msgf("proxy: unhandled net error: %v", err)
				return err // can't read
			} else {
				// 原来这里调 logger.Fatal，会直接 os.Exit(1) 把整个进程带走。
				logging.ConsoleLog.Error().Msgf("proxy: can't convert net error: %v", err)
				return err
			}
		}
		if lenOnce == 0 {
			return nil
		}
		rka.finalStr = append(rka.finalStr, tmpByte[:lenOnce]...)
	}
}

type ReadConnectionClose struct {
	p *Proxy
}

func (rcc *ReadConnectionClose) proxyReadAll() ([]byte, error) {

	var resData []byte
	tmpByte := make([]byte, 1024)
	for {
		len_once, err := rcc.p.Read(tmpByte)
		if err != nil {
			if err == io.EOF { // read all
				break
			} else {
				rcc.p.Close()
				return nil, err // can't read
			}
		}
		resData = append(resData, tmpByte[:len_once]...)
	}

	return resData, nil
}
