package proxy

import (
	"fast-https/config"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
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
)

type ReadKeepAlive struct {
	TryNum       int
	finalStr     []byte
	bodyPosition int
	res          *response.Response
	body         []byte
	p            *Proxy
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
	var contentLength int
	if res.GetHeader("Content-Length") != "" {
		// fmt.Println(res.GetHeader("Content-Length"))
		contentLength, _ = strconv.Atoi(res.GetHeader("Content-Length"))
		NeedRead := contentLength - (tmpLen - i - 4)
		rka.bodyPosition = i + 4
		rka.body = tmpData[i+4:]
		rka.res = res
		return NeedRead
	} else if res.GetHeader("Transfer-Encoding") == "chunked" {
		rka.bodyPosition = i + 4
		rka.body = tmpData[i+4:]
		rka.res = res
		return -3
	} else {
		// unkonwn
		rka.body = res.Body
		rka.res = res
		return -4
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
readAgain:

	len_once, err := rka.Read(tmpByte)

	if err != nil {
		neterr, ok := err.(net.Error)
		if ok && neterr.Timeout() {
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
	rka.finalStr = append(rka.finalStr, tmpByte[:len_once]...)
	// fmt.Println(string(ro.finalStr))
	// TRY_READ_LEN is not enough
	// if len_once == TRY_READ_LEN {
	size := rka.tryToParse(rka.finalStr)
	if size == 0 { // will not need to read
		return nil
	} else if size > 0 { // need read data in size
		if err := rka.ReadBytes(size); err != nil {
			return err
		}
	} else if size == -2 { // need read header
		// fmt.Println("invalid header")
		goto readAgain
	} else if size == -3 {
		if err := rka.parseChunked(); err != nil {
			logging.ConsoleLog.Debug().Msgf("proxy: decode chunked body: %v", err)
			return err
		}
		return nil
	} else if size == -4 {
		return nil
	}
	// }

	return nil
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
