package listener

import (
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"log"
	"net"
	"strings"
	"time"
)

// one listen port arg
type ListenInfo struct {
	Proxy      uint8 // 0 1 2
	Proxy_addr string
	Lfd        net.Listener
}

/*

{
ErrorPage:{Code:148 Path:/404.html}
LogRoot:/var/www

HttpServer:[
	{Listen:8080
		ServerName:apple.ideal.com
		Static:{Root:/var/html Index:[index.html index.htm]}
		Path:/api
		Ssl:
		Ssl_Key:
		Gzip:0
		HTTP_PROXY:
		HTTPS_PROXY:
		TCP_PROXY:
	}
	{Listen:8000
		ServerName:banana.ideal.com
		Static:{Root: Index:[]}
		Path:/api
		Ssl:
		Ssl_Key:
		Gzip:0
		HTTP_PROXY:127.0.0.1:8001
		HTTPS_PROXY:
		TCP_PROXY:
	}
	{Listen:443 ssl
		ServerName:ssl.ideal.com
		Static:{Root: Index:[]}
		Path:/api
		Ssl:/home/cert.pem
		Ssl_Key:/home/cert.key
		Gzip:0
		HTTP_PROXY:
		HTTPS_PROXY:127.0.0.1:8001
		TCP_PROXY:
	}
	{Listen:9002
		ServerName:
		Static:{Root: Index:[]}
		Path:/api
		Ssl:
		Ssl_Key:
		Gzip:0
		PROXY_TYPE:1
		PROXY_DATA:127.0.0.1:9003
	}]
}
*/

func Listen() []ListenInfo {
	lisi := make([]ListenInfo, len(config.G_config.HttpServer))
	var arr []string
	for index, each := range config.G_config.HttpServer {

		if strings.Contains(each.Listen, "ssl") {
			arr = strings.Split(each.Listen, " ")
			lisi[index].Lfd = listenssl("0.0.0.0:"+arr[0], each.Ssl, each.Ssl_Key)
			if each.HTTP_PROXY != "" {
				lisi[index].Proxy_addr = each.HTTP_PROXY
				lisi[index].Proxy = 1
			} else if each.HTTPS_PROXY != "" {
				lisi[index].Proxy_addr = each.HTTPS_PROXY
				lisi[index].Proxy = 1
			} else if each.TCP_PROXY != "" {
				lisi[index].Proxy_addr = each.TCP_PROXY
				lisi[index].Proxy = 2
			} else {
				lisi[index].Proxy = 0
			}
		} else {
			arr = strings.Split(each.Listen, " ")
			lisi[index].Lfd = listen("0.0.0.0:" + arr[0])
			if each.HTTP_PROXY != "" {
				lisi[index].Proxy_addr = each.HTTP_PROXY
				lisi[index].Proxy = 1
			} else if each.HTTPS_PROXY != "" {
				lisi[index].Proxy_addr = each.HTTPS_PROXY
				lisi[index].Proxy = 1
			} else if each.TCP_PROXY != "" {
				lisi[index].Proxy_addr = each.TCP_PROXY
				lisi[index].Proxy = 2
			} else {
				lisi[index].Proxy = 0
			}
		}
	}
	return lisi
}

func listen(laddr string) net.Listener {

	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	return listener
}

func listenssl(laddr string, cert string, key string) net.Listener {
	crt, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		log.Fatal("Error load " + cert + " cert")
	}

	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = []tls.Certificate{crt}
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader

	listener, err := tls.Listen("tcp", laddr, tlsConfig)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	return listener
}
