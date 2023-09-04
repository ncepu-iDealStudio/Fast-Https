package h2

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

// ioBuf := bufio.NewReader(each_event.Conn)
// req, err := http.ReadRequest(ioBuf)
// if err != nil {
// 	log.Println(err)
// 	break
// }
// bodyByte, _ := io.ReadAll(req.Body)
// log.Println("recv: ", string(bodyByte))
// buf := bytes.NewBuffer(nil)
// buf.WriteString("HTTP/2 200 OK\r\n")
// buf.WriteString("Content-Length: " + strconv.Itoa(len(bodyByte)) + "\r\n")
// // buf.WriteString("Connection: keep-alive\r\n")
// buf.WriteString("\r\n")
// buf.Write(bodyByte)
// buf.WriteTo(each_event.Conn)

func Test() {
	certs := []tls.Certificate{}
	crt, err := tls.LoadX509KeyPair("./config/cert/localhost.pem", "./config/cert/localhost-key.pem")
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

	for {
		conn, _ := lis.Accept()
		fmt.Println(conn.RemoteAddr().String())
		for {
			rBuf := bufio.NewReader(conn)
			frame, err := http2.ReadFrameHeader(rBuf)
			if err != nil {
				fmt.Println(err)
				conn.Close()
				break
			}
			// Decode the frame
			switch frame.Header().Type {
			case http2.FrameHeaders:
				fmt.Println("StreamID", frame.Header())

			case http2.FrameData:
				fmt.Println("data", frame.Header())
			}
		}
	}
}

func H2() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, HTTP/2!")
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: nil,
	}
	err := server.ListenAndServeTLS("./config/cert/localhost.pem", "./config/cert/localhost-key.pem")
	if err != nil {
		log.Fatal(err)
	}
}
