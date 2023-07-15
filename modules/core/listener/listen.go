package listener

import (
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"log"
	"net"
	"strings"
	"time"

	"github.com/chenhg5/collection"
)

type SSLkv struct {
	SslKey   string
	SslValue string
}

type ListenData struct {
	Proxy      uint8 // 0 1 2 3
	Proxy_addr string
	ServerName string
	Path       string
	SSL        SSLkv
	StaticRoot string
	Gzip       uint8
}

// one listen port arg
type ListenInfo struct {
	Data    []ListenData
	Lfd     net.Listener
	Port    string
	LisType uint8
}

var Lisinfos []ListenInfo

/*
{ErrorPage:
	{Code:148 Path:/404.html}
	LogRoot:/var/www
	HttpServer:[
		{Listen:8080
			ServerName:apple.ideal.com
			Static:{Root:/var/html Index:[index.html index.htm]}
			Path:/
			Ssl:
			Ssl_Key:
			Gzip:0
			PROXY_TYPE:0
			PROXY_DATA:
		}
		{Listen:8000
			ServerName:banana.ideal.com
			Static:{Root: Index:[]}
			Path:/api
			Ssl:
			Ssl_Key:
			Gzip:0
			PROXY_TYPE:1
			PROXY_DATA:127.0.0.1:8001
		}
		{Listen:443 ssl
			ServerName:ssl.ideal.com
			Static:{Root: Index:[]}
			Path:/
			Ssl:/home/cert.pem
			Ssl_Key:/home/cert.key
			Gzip:0
			PROXY_TYPE:2
			PROXY_DATA:127.0.0.1:8001
		}
		{Listen:9002
			ServerName: Static:{Root: Index:[]}
			Path:
			Ssl:
			Ssl_Key:
			Gzip:0
			PROXY_TYPE:3
			PROXY_DATA:127.0.0.1:9003
		}
	]}

*/

func ProcessPorts() {
	var Ports []string
	lis_temp := ListenInfo{}
	for _, each := range config.G_config.HttpServer {

		arr := strings.Split(each.Listen, " ")
		if !collection.Collect(Ports).Contains(arr[0]) {

			Ports = append(Ports, arr[0])

			lis_temp.Data = nil
			lis_temp.Lfd = nil
			if strings.Contains(each.Listen, "ssl") {
				lis_temp.LisType = 1 // ssl
			} else if strings.Contains(each.Listen, "tcp") {
				lis_temp.LisType = 2 // tcp
			} else {
				lis_temp.LisType = 0
			}
			lis_temp.Port = arr[0]
			Lisinfos = append(Lisinfos, lis_temp)
		}

	}
}

func ProcessData() {
	for _, each := range config.G_config.HttpServer {
		for index, item := range Lisinfos {
			num := strings.Split(each.Listen, " ")[0]
			if item.Port == num {
				data := ListenData{}
				data.Path = each.Path
				if item.Port == "80" || item.Port == "443" {
					data.ServerName = each.ServerName
				} else {
					data.ServerName = each.ServerName + ":" + item.Port
				}
				data.Proxy = each.PROXY_TYPE
				data.StaticRoot = each.Static.Root
				data.Proxy_addr = each.PROXY_DATA
				data.SSL = SSLkv{each.Ssl, each.Ssl_Key}
				data.Gzip = each.Gzip

				Lisinfos[index].Data = append(item.Data, data)
			}
		}
	}
}

func Listen() []ListenInfo {
	ProcessPorts()
	ProcessData()

	// [{[{0  apple.ideal.com /static { } /home/pzc/Project/fast-https/static}] <nil> 8080 0}
	// {[{1 192.168.11.236:5000 banana.ideal.com /api { } }] <nil> 8000 0}
	// {[{2 192.168.11.236:5000 ssl.ideal.com / {/home/pzc/Project/fast-https/config/cert/apple.ideal.com.pem /home/pzc/Project/fast-https/config/cert/apple.ideal.com-key.pem} }] <nil> 443 1}
	// {[{3 127.0.0.1:9003   { } }] <nil> 9002 0}
	// ]

	for index, each := range Lisinfos {
		if each.LisType == 1 {
			Lisinfos[index].Lfd = listenssl("0.0.0.0:"+each.Port, each.Data)
		} else {
			Lisinfos[index].Lfd = listen("0.0.0.0:" + each.Port)
		}
	}
	return Lisinfos
}

func listen(laddr string) net.Listener {
	log.Println("[Listener:]listen", laddr)

	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	return listener
}

func listenssl(laddr string, lisdata []ListenData) net.Listener {
	log.Println("[Listener:]listen", laddr)
	certs := []tls.Certificate{}
	for _, item := range lisdata {
		crt, err := tls.LoadX509KeyPair(item.SSL.SslKey, item.SSL.SslValue)
		if err != nil {
			log.Fatal("Error load " + item.SSL.SslKey + " cert")
		}
		certs = append(certs, crt)
		log.Println("[Listener:]Load ssl file", item.ServerName)
	}
	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = certs
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader

	listener, err := tls.Listen("tcp", laddr, tlsConfig)
	if err != nil {
		log.Fatal("Error starting the server:", err)
	}
	return listener
}
