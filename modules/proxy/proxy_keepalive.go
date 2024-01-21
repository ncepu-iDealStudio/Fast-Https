package proxy

import (
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	TRY_READ_LEN   = 2048
	READ_BYTES_LEN = 4096
)

type ReadOnce struct {
	TryNum       int
	finalStr     []byte
	bodyPosition int
	body         []byte
	Type         int
	ProxyConn    net.Conn
	ProxyTlsConn *tls.Conn
}

/*
  - return -1 str is too short
    return -2 parse failed no such Header "Content-Length"
  - if parse successed, return a number that need to be read.
*/
func (ro *ReadOnce) tryToParse(tmpData []byte) int {

	tmpLen := len(tmpData)
	if tmpLen < 4 {
		// str is too short
		return -1 // parse failed!
	}
	// fmt.Println(tmpData)

	var i int
	i = strings.Index(string(tmpData), "\r\n\r\n")

	if i == -1 {
		// parse failed! "no \r\n\r\n"
		// caller need call this again maybe
		ro.TryNum = ro.TryNum + 1
		return -2
	}

	res := response.ResponseInit()
	res.HttpResParse(string(tmpData))
	var contentLength int
	if res.GetHeader("Content-Length") != "" {
		// fmt.Println(res.GetHeader("Content-Length"))
		contentLength, _ = strconv.Atoi(res.GetHeader("Content-Length"))
		NeedRead := contentLength - (tmpLen - i - 4)
		return NeedRead
	} else if res.GetHeader("Transfer-Encoding") == "chunked" {
		ro.bodyPosition = i + 4
		ro.body = tmpData[i+4:]
		return -3
	} else {
		// unkonwn
		return -100
	}

}

func (ro *ReadOnce) parseChunked() {
	// 2b81\r\n
	// dddddddddd
	// 0\r\n
	// \r\n
	var p int
	for {
		if p = strings.Index(string(ro.body), "0\r\n"); p != -1 { // last block
			return
		} else {
			// if p = strings.Index(string(ro.body), "\r\n"); p == -1 { // body like this  "2b8" or "2b81\r"
			// }
			lastBuf := make([]byte, READ_BYTES_LEN)
			n, err := ro.Read(lastBuf)
			if err != nil {
				fmt.Println("parseChuncked failed", err.Error())
				return
			} else {
				ro.body = append(ro.body, lastBuf[:n]...)
			}

		}
	}
}

func (ro *ReadOnce) Read(data []byte) (int, error) {
	if ro.Type == config.PROXY_HTTP {
		return ro.ProxyConn.Read(data)
	} else if ro.Type == config.PROXY_HTTPS {
		return ro.ProxyTlsConn.Read(data)
	} else {
		message.PrintErr("--proxy read error")
		return 0, nil
	}
}

func (ro *ReadOnce) ReadBytes(size int) {
	totalLen := size
	for {
		onceSize := READ_BYTES_LEN - totalLen
		if onceSize > 0 {
			lastBuf := make([]byte, READ_BYTES_LEN-onceSize)
			lastLen, err := ro.Read(lastBuf)
			if err != nil || lastLen != READ_BYTES_LEN-onceSize {
				message.PrintErr("ReadBytes error", err)
			}
			ro.finalStr = append(ro.finalStr, lastBuf...)
			return
		} else {
			tmpBuf := make([]byte, READ_BYTES_LEN)
			tempLen, err := ro.Read(tmpBuf)
			if err != nil || tempLen != READ_BYTES_LEN {
				message.PrintErr("ReadBytes error", err)
			}
			ro.finalStr = append(ro.finalStr, tmpBuf...)
		}
		totalLen -= READ_BYTES_LEN
	}
}

func (ro *ReadOnce) proxyReadOnce(ev *core.Event) error {

	tmpByte := make([]byte, TRY_READ_LEN)
readAgain:

	len_once, err := ro.Read(tmpByte)

	if err != nil {
		return err // can't read
	}
	ro.finalStr = append(ro.finalStr, tmpByte[:len_once]...)
	// fmt.Println(string(ro.finalStr))
	// TRY_READ_LEN is not enough
	// if len_once == TRY_READ_LEN {
	size := ro.tryToParse(ro.finalStr)
	if size == 0 { // will not need to read
		return nil
	} else if size > 0 { // need read data in size
		ro.ReadBytes(size)
	} else if size == -2 { // need read header
		// fmt.Println("invalid header")
		goto readAgain
	} else if size == -3 {
		// ro.parseChunked()
		return errors.New("response con not parse chuncked")
	} else {
		return errors.New("response parse error")
	}
	// }

	return nil
}
