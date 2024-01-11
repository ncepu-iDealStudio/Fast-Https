/**
* @Author: yizhigopher
* @Description：just one example to show how to handle http2 byte flow
* @File: h2_example
* @Version:1.0.0
* @Date:2023/10/29 21:13:25
 */

package h2

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/http2/hpack"
)

const _crt = "./config/cert/localhost.pem"
const _key = "./config/cert/localhost-key.pem"

const (
	CONN_PREFACE   = 24
	FRAME_HEAD_LEN = 9
)

/* Http2 HeaderFrame Structure
+----------------------------------------------+
|                Length(24)                    |
+--------------+---------------+---------------+
|    Type(8)   |    Flags(8)   |
+--------------+---------------+-------------------------------+
|R|                 Stream Identifier(31)                      |
+=+============================================================+
|                   HeaderFrame Payload (0...)                     ...
+--------------------------------------------------------------+
*/

// FrameType define the type of http2 frame
type FrameType uint8

const (
	FrameData         FrameType = 0x0
	FrameHeaders      FrameType = 0x1
	FramePriority     FrameType = 0x2
	FrameRSTStream    FrameType = 0x3
	FrameSettings     FrameType = 0x4
	FramePushPromise  FrameType = 0x5
	FramePing         FrameType = 0x6
	FrameGoAway       FrameType = 0x7
	FrameWindowUpdate FrameType = 0x8
	FrameContinuation FrameType = 0x9
)

type FrameHeader struct {
	Length   uint32
	Type     FrameType
	Flags    uint8
	StreamID uint32
}

func ReadHeader(buf []byte, conn net.Conn) (FrameHeader, error) {
	_, err := conn.Read(buf)
	if err != nil {
		return FrameHeader{}, err
	}
	return FrameHeader{
		Length:   (uint32(buf[0])<<16 | uint32(buf[1])<<8 | uint32(buf[2])),
		Type:     FrameType(buf[3]),
		Flags:    buf[4],
		StreamID: binary.BigEndian.Uint32(buf[5:]) & (1<<31 - 1),
	}, nil
}

// HeaderFrame 对应类型为HEADERS的数据帧
type HeaderFrame struct {
	FrameHeader
	Payload map[string]string
}

// ReadPayload 实现了根据请求头部的长度字段，读取负载信息，并加载信息到HeaderFrame上
func (h *HeaderFrame) ReadPayload(conn net.Conn) error {
	payload := make([]byte, h.Length)
	_, err := conn.Read(payload)
	if err != nil {
		return err
	}
	decoder := hpack.NewDecoder(4096, nil)
	full, _ := decoder.DecodeFull(payload)
	fmt.Println("Decode Head HeaderFrame PayLoad=", full)
	for _, headerField := range full {
		h.Payload[headerField.Name] = headerField.Value
	}
	fmt.Println("Unmarshal Head HeaderFrame to HeaderFrame=", h.Payload)
	return nil
}

// Response 表示想连接中写信息
func (h *HeaderFrame) Response(conn net.Conn, resp []byte) (err error) {
	_, err = conn.Write(resp)
	return
}

// handleOtherFrame TODO: Implement other types of frames handling
func handleOtherFrame(typename string, conn net.Conn, length uint32) {
	// just read
	payload := make([]byte, length)
	conn.Read(payload)
	fmt.Printf("hanle %s PayLoad=%v\n", typename, payload)
}

// TODO: we should implement other frame type
// eg :type DataFrame struct {
//	 	...
// }

func Example() {
	certs := []tls.Certificate{}
	crt, err := tls.LoadX509KeyPair(_crt, _key)
	if err != nil {
		fmt.Println("load err")
	}
	certs = append(certs, crt)
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = certs
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader
	tlsConfig.NextProtos = append(tlsConfig.NextProtos, "h1")
	tlsConfig.NextProtos = append(tlsConfig.NextProtos, "h2")
	tlsConfig.NextProtos = append(tlsConfig.NextProtos, "h2c")

	lis, err := tls.Listen("tcp", "0.0.0.0:5555", tlsConfig)
	if err != nil {
		fmt.Println("listen err")
	}

	conn, _ := lis.Accept()
	defer conn.Close()
	fmt.Println(conn.RemoteAddr().String())

	// 读取http2连接的连接前奏
	connectionPreface := make([]byte, CONN_PREFACE)
	conn.Read(connectionPreface)
	fmt.Printf("connection-preface=\n%s", string(connectionPreface))
	if string(connectionPreface) == "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n" {
		// we should send setting frame(empty is ok)
		setting_frame := []byte{0, 0, 0, 4, 0, 0, 0, 0, 0}
		conn.Write(setting_frame)
	}

	// 处理来自客户端的请求
	for {
		buf := make([]byte, FRAME_HEAD_LEN)
		frameHeader, _ := ReadHeader(buf, conn)
		switch frameHeader.Type {
		case FrameHeaders:
			frame := HeaderFrame{frameHeader, make(map[string]string)}
			err = frame.ReadPayload(conn)
			if err != nil {
				fmt.Println(err)
				continue
			}
			resp := []byte{0, 0, 1, 0, 0, 0, 0, 0, 0, 2}
			err = frame.Response(conn, resp)
			if err != nil {
				fmt.Println(err)
				continue
			}
		case FrameData:
			handleOtherFrame("FrameData", conn, frameHeader.Length)
		default:
			handleOtherFrame("OtherFrame", conn, frameHeader.Length)
		}
	}
}
