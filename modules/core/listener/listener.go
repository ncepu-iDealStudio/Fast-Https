package listener

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"fast-https/utils/logger"
	"fast-https/utils/message"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/chenhg5/collection"
)

// donmain cert and key
type SSLkv struct {
	SslKey   string
	SslValue string
}

type Try struct {
	UriRe *regexp.Regexp
	Files []string
	Next  string
}

// struct like confgure "location"
type ListenCfg struct {
	ID         int
	SSL        SSLkv
	ServerName string
	Path       string
	PathRe     *regexp.Regexp
	Trys       []Try

	// 10 is dev mod
	Type           uint16 // 0 1 2 3 4
	ProxyAddr      string
	ProxySetHeader []config.Header
	ProxyCache     config.Cache
	AppFireWall    []string
	ReWrite        string

	Limit config.PathLimit
	Auth  config.PathAuth

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
}

var GLisinfos []Listener

func FindPorts() []string {
	var Ports []string
	for _, each := range config.GConfig.Servers {
		arr := strings.Split(each.Listen, " ")
		if !collection.Collect(Ports).Contains(arr[0]) {
			Ports = append(Ports, arr[0])
		}
	}
	return Ports
}

func FindOldPorts() []string {
	var Ports []string
	for _, item := range GLisinfos {
		if !collection.Collect(Ports).Contains(item.Port) {
			// always true
			Ports = append(Ports, item.Port)
		} else {
			logger.Fatal("find current ports error")
		}
	}
	return Ports
}

// sort confgure form "listen"
// fill port and listen type
func SortByPort(lisInfos *[]Listener) {
	var Ports []string
	lis_temp := Listener{}
	for _, each := range config.GConfig.Servers {

		arr := strings.Split(each.Listen, " ")
		if !collection.Collect(Ports).Contains(arr[0]) {
			Ports = append(Ports, arr[0])

			lis_temp.Cfg = nil
			lis_temp.Lfd = nil
			lis_temp.HostMap = make(map[string][]ListenCfg)
			// lis_temp.Limit = each.Limit
			if strings.Contains(each.Listen, "ssl") {
				lis_temp.LisType = 1 // ssl
				if strings.Contains(each.Listen, "h2") {
					lis_temp.LisType = 10
				}
			} else if strings.Contains(each.Listen, "tcp") {
				lis_temp.LisType = 2 // tcp proxy
			} else {
				lis_temp.LisType = 0
			}
			lis_temp.Port = arr[0]
			*lisInfos = append(*lisInfos, lis_temp)
		}

	}
}

func SortBySpecificPorts(ports []string, lisInfos *[]Listener) {
	for _, port := range ports {
		for _, each := range config.GConfig.Servers { // new config
			if port == strings.Split(each.Listen, " ")[0] {
				lis_temp := Listener{}
				lis_temp.Cfg = nil
				lis_temp.Lfd = nil
				lis_temp.HostMap = make(map[string][]ListenCfg)
				if strings.Contains(each.Listen, "ssl") {
					lis_temp.LisType = 1 // ssl
					if strings.Contains(each.Listen, "h2") {
						// h2 not support in this branch
						// logger.Fatal("h2 not support in this branch")
						lis_temp.LisType = 10
					}
				} else if strings.Contains(each.Listen, "tcp") {
					lis_temp.LisType = 2 // tcp proxy
				} else {
					lis_temp.LisType = 0
				}
				lis_temp.Port = strings.Split(each.Listen, " ")[0]
				*lisInfos = append(*lisInfos, lis_temp)
				break
			}
		}
	}

}

// sort configure from "path"
func processListenData(lisInfos *[]Listener) {
	Id := 0
	for _, server := range config.GConfig.Servers {
		for _, paths := range server.Path {

			for index, eachlisten := range *lisInfos {
				listen := strings.Split(server.Listen, " ")[0]
				if eachlisten.Port == listen {
					data := ListenCfg{}
					data.ID = Id
					data.Path = paths.PathName
					data.PathRe = regexp.MustCompile(paths.PathName)
					for _, item := range paths.Trys {
						try := Try{
							UriRe: regexp.MustCompile(item.Uri),
							Files: item.Files,
							Next:  item.Next,
						}
						data.Trys = append(data.Trys, try)
					}
					data.ServerName = server.ServerName + ":" + eachlisten.Port
					data.Type = paths.PathType
					data.ProxyAddr = paths.ProxyData
					data.ProxySetHeader = paths.ProxySetHeader
					data.AppFireWall = paths.AppFireWall
					data.Limit = paths.Limit
					data.Auth = paths.Auth
					data.StaticRoot = paths.Root
					data.StaticIndex = paths.Index
					data.SSL = SSLkv{server.SSLCertificate, server.SSLCertificateKey}
					data.Zip = paths.Zip
					data.ReWrite = paths.Rewrite
					data.ProxyCache = paths.ProxyCache
					listener := &(*lisInfos)[index]
					listener.Cfg = append(eachlisten.Cfg, data)
					Id = Id + 1
				}
			}
		}
	}
}

func processHostMap(lisInfos *[]Listener) {
	for _, eachPort := range *lisInfos {
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
func ListenWithCfg() []Listener {
	var CurrLisinfos []Listener
	SortByPort(&CurrLisinfos)
	processListenData(&CurrLisinfos)
	processHostMap(&CurrLisinfos)

	for index, each := range CurrLisinfos {
		if each.LisType == 1 || each.LisType == 10 {
			CurrLisinfos[index].Lfd = listenSsl("0.0.0.0:"+each.Port, each.Cfg, true)
		} else {
			CurrLisinfos[index].Lfd = listenTcp("0.0.0.0:"+each.Port, true)
		}
	}

	GLisinfos = CurrLisinfos
	return CurrLisinfos
}

/*
	func comparePorts(curr_ports, new_ports []string) (added, removed, common []string) {
		// Create a map to store the current set of ports
		currPortsMap := make(map[string]struct{})
		for _, port := range curr_ports {
			currPortsMap[port] = struct{}{}
		}

		// Create a map to store the new set of ports
		newPortsMap := make(map[string]struct{})
		for _, port := range new_ports {
			newPortsMap[port] = struct{}{}
		}

		// Find the added ports
		for port := range newPortsMap {
			if _, found := currPortsMap[port]; !found {
				added = append(added, port)
			}
		}

		// Find the removed ports
		for port := range currPortsMap {
			if _, found := newPortsMap[port]; !found {
				removed = append(removed, port)
			} else {
				// If the port exists in both sets, add it to the common slice
				common = append(common, port)
			}
		}

		return added, removed, common
	}
*/
func comparePorts(curr_ports, new_ports []string) (added, removed, common []string) {
	// Find the added ports
	for _, newPort := range new_ports {
		found := false
		for _, currPort := range curr_ports {
			if newPort == currPort {
				found = true
				break
			}
		}
		if !found {
			added = append(added, newPort)
		}
	}

	// Find the removed ports and common ports
	for _, currPort := range curr_ports {
		found := false
		for _, newPort := range new_ports {
			if currPort == newPort {
				found = true
				common = append(common, currPort)
				break
			}
		}
		if !found {
			removed = append(removed, currPort)
		}
	}

	return added, removed, common
}

// tcp listen
func listenTcp(laddr string, reuse bool) net.Listener {
	message.PrintInfo("Listen ", laddr)

	if reuse {
		cfg := net.ListenConfig{
			Control: ReuseCallBack,
		}
		listener, err := cfg.Listen(context.Background(), "tcp", laddr)
		if err != nil {
			logger.Fatal("Error listen: %v", err)
		}
		return listener
	} else {
		listener, err := net.Listen("tcp", laddr)
		if err != nil {
			logger.Fatal("Error listen: %v", err)
		}
		return listener
	}
}

// ssl listen
func listenSsl(laddr string, lisdata []ListenCfg, reuse bool) net.Listener {
	certs := []tls.Certificate{}
	var servernames []string

	for _, item := range lisdata {
		if !collection.Collect(servernames).Contains(item.ServerName) {
			crt, err := tls.LoadX509KeyPair(item.SSL.SslKey, item.SSL.SslValue)
			if err != nil {
				logger.Debug("Error load cert: %s" + item.SSL.SslKey)
			}
			certs = append(certs, crt)
			logger.Info("Automatically load %s certificate", item.ServerName)
		}
		servernames = append(servernames, item.ServerName)
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"h2"},
		Certificates: certs,
		Time:         time.Now,
		Rand:         rand.Reader,
	}

	tcpListener := listenTcp(laddr, reuse)

	// 在TCP监听器上叠加TLS
	listener := tls.NewListener(tcpListener, tlsConfig)

	return listener
}
