package proxy

import (
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"net"
	"strconv"
	"strings"
)

const (
	TRY_READ_LEN   = 2048
	READ_BYTES_LEN = 4096
)

type ReadOnce struct {
	TryNum    int
	finalStr  []byte
	ProxyConn net.Conn
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
	} else if res.GetHeader("Transfer-Encoding") == "chunked" {

	}

	NeedRead := contentLength - (tmpLen - i - 4)
	return NeedRead
}

func parseChunked() {
	// 2b81\r\n dddddddddd

	// 0\r\n
	// \r\n
}

func (ro *ReadOnce) ReadBytes(size int) {
	totalLen := size
	for {
		onceSize := READ_BYTES_LEN - totalLen
		if onceSize > 0 {
			lastBuf := make([]byte, READ_BYTES_LEN-onceSize)
			lastLen, err := ro.ProxyConn.Read(lastBuf)
			if err != nil || lastLen != READ_BYTES_LEN-onceSize {
				message.PrintErr("ReadBytes error", err)
			}
			ro.finalStr = append(ro.finalStr, lastBuf...)
			return
		} else {
			tmpBuf := make([]byte, READ_BYTES_LEN)
			tempLen, err := ro.ProxyConn.Read(tmpBuf)
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

	len_once, err := ro.ProxyConn.Read(tmpByte)
	if err != nil {
		return err // can't read
	}
	ro.finalStr = append(ro.finalStr, tmpByte[:len_once]...)

	// TRY_READ_LEN is not enough
	if len_once == TRY_READ_LEN {
		size := ro.tryToParse(ro.finalStr)
		if size > 0 {
			ro.ReadBytes(size)
		} else if size == -2 {
			// fmt.Println("invalid header")
			goto readAgain
		}
	}

	return nil
}
