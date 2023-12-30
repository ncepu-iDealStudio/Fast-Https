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

// struct like confgure "location"
type ListenCfg struct {
	SSL        SSLkv
	ServerName string
	Path       string

	Type           uint16 // 0 1 2 3
	Proxy_addr     string
	ProxySetHeader []config.Header
	ProxyCache     config.Cache

	Limit config.PathLimit

	StaticRoot  string
	StaticIndex []string
	Zip         uint16
}

// one listen port arg
type Listener struct {
	HostMap map[string]([]ListenCfg)
	Cfg     []ListenCfg
	Lfd     net.Listener
	Port    string
	LisType uint8
	Limit   config.ServerLimit
}

var Lisinfos []Listener

// sort confgure form "listen"
func Process_ports() []string {
	var Ports []string
	lis_temp := Listener{}
	for _, each := range config.GConfig.Servers {

		arr := strings.Split(each.Listen, " ")
		if !collection.Collect(Ports).Contains(arr[0]) {
			Ports = append(Ports, arr[0])

			lis_temp.Cfg = nil
			lis_temp.Lfd = nil
			lis_temp.HostMap = make(map[string][]ListenCfg)
			lis_temp.Limit = each.Limit
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
func process_listen_data() {
	for _, server := range config.GConfig.Servers {
		for _, paths := range server.Path {

			for index, eachlisten := range Lisinfos {
				listen := strings.Split(server.Listen, " ")[0]
				if eachlisten.Port == listen {
					data := ListenCfg{}
					data.Path = paths.PathName
					data.ServerName = server.ServerName + ":" + eachlisten.Port
					data.Type = paths.PathType
					data.Proxy_addr = paths.ProxyData
					data.ProxySetHeader = paths.ProxySetHeader
					data.Limit = paths.Limit
					data.StaticRoot = paths.Root
					data.StaticIndex = paths.Index
					data.SSL = SSLkv{server.SSLCertificate, server.SSLCertificateKey}
					data.Zip = paths.Zip
					data.ProxyCache = paths.ProxyCache
					Lisinfos[index].Cfg = append(eachlisten.Cfg, data)
				}
			}
		}
	}
}

func process_host_map() {
	for _, eachPort := range Lisinfos {
		processEachPort(eachPort)
	}
}

func processEachPort(lisPort Listener) {
	var nameArray []string
	for _, item := range lisPort.Cfg {
		if !collection.Collect(nameArray).Contains(item.ServerName) {
			nameArray = append(nameArray, item.ServerName)
		}
	}
	for _, name := range nameArray {
		var tempListenCfgArray []ListenCfg
		for _, item := range lisPort.Cfg {
			if name == item.ServerName {
				tempListenCfgArray = append(tempListenCfgArray, item)
			}
		}
		lisPort.HostMap[name] = tempListenCfgArray
	}
}

// listen some ports
func Listen() []Listener {
	Process_ports()
	process_listen_data()
	process_host_map()

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
