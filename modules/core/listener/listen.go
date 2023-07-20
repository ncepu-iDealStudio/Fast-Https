package listener

import (
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"fmt"
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
	Proxy       uint8 // 0 1 2 3
	Proxy_addr  string
	ServerName  string
	Path        string
	SSL         SSLkv
	StaticRoot  string
	StaticIndex []string
	Gzip        uint8
}

// one listen port arg
type ListenInfo struct {
	Data    []ListenData
	Lfd     net.Listener
	Port    string
	LisType uint8
}

var Lisinfos []ListenInfo

func ProcessPorts() {
	var Ports []string
	lis_temp := ListenInfo{}
	for _, each := range config.G_config.Servers {

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
	fmt.Println("config.G_config.Servers length:", len(config.G_config.Servers))
	for _, server := range config.G_config.Servers {
		for _, paths := range server.Path {
			for index, eachlisten := range Lisinfos {
				listen := strings.Split(server.Listen, " ")[0]
				if eachlisten.Port == listen {
					data := ListenData{}
					data.Path = paths.PathName
					if eachlisten.Port == "80" || eachlisten.Port == "443" {
						data.ServerName = server.ServerName
					} else {
						data.ServerName = server.ServerName + ":" + eachlisten.Port
					}
					data.Proxy = paths.PathType
					data.StaticRoot = paths.Root
					data.StaticIndex = paths.Index
					data.Proxy_addr = paths.ProxyData
					data.SSL = SSLkv{server.SSLCertificate, server.SSLCertificateKey}
					data.Gzip = paths.Zip
					Lisinfos[index].Data = append(eachlisten.Data, data)
				}
			}
		}
	}
}

func Listen() []ListenInfo {
	ProcessPorts()
	ProcessData()

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

	var servernames []string

	for _, item := range lisdata {

		if !collection.Collect(servernames).Contains(item.ServerName) {
			crt, err := tls.LoadX509KeyPair(item.SSL.SslKey, item.SSL.SslValue)
			if err != nil {
				log.Fatal("Error load " + item.SSL.SslKey + " cert")
			}
			certs = append(certs, crt)
			log.Println("[Listener:]----", item.ServerName, "start ssl listen")
		}
		servernames = append(servernames, item.ServerName)

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
