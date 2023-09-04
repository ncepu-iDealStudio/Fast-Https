package listener

import (
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"fast-https/utils/message"
	"net"
	"strings"
	"time"

	"github.com/chenhg5/collection"
)

const (
	STATIC_EVENT = 0
)

// donmain cert and key
type SSLkv struct {
	SslKey   string
	SslValue string
}

// struct like confgure "path"
type ListenCfg struct {
	Proxy          uint8 // 0 1 2 3
	Proxy_addr     string
	ProxySetHeader []config.Header
	ServerName     string
	Path           string
	SSL            SSLkv
	StaticRoot     string
	StaticIndex    []string
	Zip            uint8
}

// one listen port arg
type ListenInfo struct {
	Cfg     []ListenCfg
	Lfd     net.Listener
	Port    string
	LisType uint8
}

var Lisinfos []ListenInfo

// sort confgure form "listen"
func Process_ports() []string {
	var Ports []string
	lis_temp := ListenInfo{}
	for _, each := range config.GConfig.Servers {

		arr := strings.Split(each.Listen, " ")
		if !collection.Collect(Ports).Contains(arr[0]) {

			Ports = append(Ports, arr[0])

			lis_temp.Cfg = nil
			lis_temp.Lfd = nil
			if strings.Contains(each.Listen, "ssl") {
				lis_temp.LisType = 1 // ssl
			} else if strings.Contains(each.Listen, "tcp") {
				lis_temp.LisType = 2 // tcp proxy
			} else {
				lis_temp.LisType = 0
			}
			lis_temp.Port = arr[0]
			Lisinfos = append(Lisinfos, lis_temp)
		}

	}
	return Ports
}

// sort confgure from "path"
func Process_listen_data() {
	for _, server := range config.GConfig.Servers {
		for _, paths := range server.Path {

			for index, eachlisten := range Lisinfos {
				listen := strings.Split(server.Listen, " ")[0]
				if eachlisten.Port == listen {
					data := ListenCfg{}
					data.Path = paths.PathName
					data.ServerName = server.ServerName + ":" + eachlisten.Port
					data.Proxy = paths.PathType
					data.Proxy_addr = paths.ProxyData
					data.ProxySetHeader = paths.ProxySetHeader
					data.StaticRoot = paths.Root
					data.StaticIndex = paths.Index
					data.SSL = SSLkv{server.SSLCertificate, server.SSLCertificateKey}
					data.Zip = paths.Zip
					Lisinfos[index].Cfg = append(eachlisten.Cfg, data)
				}
			}
		}
	}
}

// listen some ports
func Listen() []ListenInfo {
	Process_ports()
	Process_listen_data()

	for index, each := range Lisinfos {
		if each.LisType == 1 {
			Lisinfos[index].Lfd = listen_ssl("0.0.0.0:"+each.Port, each.Cfg)
		} else {
			Lisinfos[index].Lfd = listen_tcp("0.0.0.0:" + each.Port)
		}
	}
	return Lisinfos
}

// tcp listen
func listen_tcp(laddr string) net.Listener {
	message.PrintInfo("Listen ", laddr)

	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		message.PrintErr("Error listen_tcp :", err)
	}
	return listener
}

// ssl listen
func listen_ssl(laddr string, lisdata []ListenCfg) net.Listener {
	message.PrintInfo("listen ", laddr)
	certs := []tls.Certificate{}
	var servernames []string

	for _, item := range lisdata {
		if !collection.Collect(servernames).Contains(item.ServerName) {
			crt, err := tls.LoadX509KeyPair(item.SSL.SslKey, item.SSL.SslValue)
			if err != nil {
				message.PrintErr("Error load cert: " + item.SSL.SslKey)
			}
			certs = append(certs, crt)
			message.PrintInfo("Automatically load " + item.ServerName + " certificate")
		}
		servernames = append(servernames, item.ServerName)
	}

	tlsConfig := &tls.Config{}
	tlsConfig.Certificates = certs
	tlsConfig.Time = time.Now
	tlsConfig.Rand = rand.Reader
	listener, err := tls.Listen("tcp", laddr, tlsConfig)
	if err != nil {
		message.PrintErr("Error listen_ssl: ", err)
	}
	return listener
}
