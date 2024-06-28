package proxy

import (
	"fast-https/config"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/Jxck/logger"
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

func Parse(data string) ([]byte, int64) {
	startIndex := 0
	var after []byte
	var total_len int64
	for {
		// 查找长度字段的结束位置
		endLengthIndex := strings.Index(data[startIndex:], "\r\n")
		if endLengthIndex == -1 {
			break // 没有找到长度字段，退出循环
		}
		endLengthIndex += startIndex // 更新结束位置

		// 获取长度字段
		lengthStr := data[startIndex:endLengthIndex]
		length, err := strconv.ParseInt(lengthStr, 16, 64)
		if err != nil {
			// fmt.Println("解析长度失败:", err, "length string:", lengthStr)
			return after, total_len
		} else {
			// fmt.Println(length)
			total_len = total_len + length
		}

		if length == 0 {
			break
		}

		// 计算数据区的结束位置
		startDataIndex := endLengthIndex + 2
		endDataIndex := startDataIndex + int(length)

		// 获取数据部分
		dataPart := data[startDataIndex:endDataIndex]

		// 打印数据内容
		// fmt.Println("数据内容:", dataPart)
		after = append(after, []byte(dataPart)...)

		// 更新起始位置，准备处理下一个数据区
		startIndex = endDataIndex + 2
	}

	return nil, 0
}

func (rka *ReadKeepAlive) parseChunked() {
	var p int
	// var after_body []byte
	// var total_length int64
	for {
		if p = strings.Index(string(rka.body), "0\r\n\r\n"); p != -1 { // last block

			// data := []byte("2\r\n11\r\n20\r\n22222222222222222222222222222222\r\n5\r\n33333\r\n0\r\n\r\n")
			_, _ = Parse(string(rka.body))
			// after_body, total_length = Parse(string(ro.body))
			// fmt.Println(total_length)

			// ro.res.DelHeader("Transfer-Encoding")
			// ro.res.Headers["Content-Length"] = strconv.Itoa(int(total_length))
			rka.res.Body = rka.body // after_body
			rka.finalStr = rka.res.GenerateResponse()
			return
		} else {
			// if p = strings.Index(string(ro.body), "\r\n"); p == -1 { // body like this  "2b8" or "2b81\r"
			// }
			lastBuf := make([]byte, CHUNCKED_BODY_SIZE)
			n, err := rka.Read(lastBuf)
			if err != nil {
				fmt.Println("parseChuncked failed", err.Error())
				return
			} else {
				rka.body = append(rka.body, lastBuf[:n]...)
			}

		}
	}
}

func (rka *ReadKeepAlive) Read(data []byte) (int, error) {
	if rka.p.ProxyType == config.PROXY_HTTP {
		return rka.p.ProxyConn.Read(data)
	} else if rka.p.ProxyType == config.PROXY_HTTPS {
		return rka.p.ProxyTlsConn.Read(data)
	} else {
		message.PrintErr("--proxy read error")
		return 0, nil
	}
}

// debug by zhangjiayue
func (rka *ReadKeepAlive) ReadBytes(size int) {
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
				logger.Debug("read bytes time out")
				break
			}
		}
		rka.finalStr = append(rka.finalStr, tmpBuf[:tempLen]...)
		rka.body = append(rka.body, tmpBuf[:tempLen]...)
		totalLen -= tempLen
	}
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
			logger.Debug("unhandled error")
			return err // can't read
		} else {
			logger.Fatal("can't convent net error")
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
		rka.ReadBytes(size)
	} else if size == -2 { // need read header
		// fmt.Println("invalid header")
		goto readAgain
	} else if size == -3 {
		rka.parseChunked()
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
