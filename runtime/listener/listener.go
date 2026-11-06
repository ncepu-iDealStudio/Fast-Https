// Package listener 管理 TCP/TLS 监听、证书加载与监听重载。
package listener

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"fast-https/config"
	"fast-https/infra/logging"
	"net"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	// Aliases 是 server_name 多域名写法下除主域名外的其他域名（含端口后缀），
	// 与 ServerName 指向同一组 locations。单域名配置为空切片。
	Aliases []string
	// ExtraCerts 是多域名 server 块除主域名证书外的其他域名证书路径，
	// loadCerts 会把 SSL 与 ExtraCerts 全部加载，SNI 按域名匹配。
	ExtraCerts []SSLkv
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

	StaticRoot    string
	StaticIndex   []string
	Zip           uint16
	ErrorNotFound string
	ErrorProxy    string

	// AllowMethods 非空时限定该 location 允许的 HTTP 方法，越界返回 405。
	AllowMethods []string
}

// listenerSnapshot 是请求路径读取的配置快照。
// reload 热更新时整体替换快照指针，在途连接读到的永远是完整的
// 新配置或完整的旧配置，而不是正在被改写的字段。
type listenerSnapshot struct {
	HostMap map[string][]ListenCfg
	Cfg     []ListenCfg
}

// one listen port arg
type Listener struct {
	// HostMap/Cfg 是构造期工作字段，由启动与 reload 协程填写；
	// 请求路径读取请使用 Hosts()/Cfgs()，它们走发布的快照。
	HostMap map[string]([]ListenCfg)
	Cfg     []ListenCfg
	Lfd     net.Listener
	Port    string
	LisType uint8

	Ctx    context.Context
	Cancel context.CancelFunc

	// alpnACMESnapshot 记录建立这个 TLS 监听器时是否通告 acme-tls/1。
	// tls.Config 建好后 NextProtos 改不了，reload 只能靠它判断要不要
	// 重建这一个端口（ACME 开关 tls-alpn-01 时会翻转）。
	alpnACMESnapshot bool

	certs *certSet
	cur   atomic.Pointer[listenerSnapshot]
}

// publish 把构造好的 Cfg/HostMap 发布为请求路径可见的快照。
func (l *Listener) publish() {
	l.cur.Store(&listenerSnapshot{HostMap: l.HostMap, Cfg: l.Cfg})
}

// openListener 建立该端口的 socket。ssl 端口同时记录建 tls.Config 时的
// ALPN 通告位，reload 据此判断是否需要重建（NextProtos 建好后改不了）。
func (l *Listener) openListener() {
	if l.LisType == 1 || l.LisType == 10 {
		l.certs = newCertSet()
		l.alpnACMESnapshot = TLSALPNEnabled()
		l.Lfd = listenSsl("0.0.0.0:"+l.Port, l.certs, l.Cfg, l.LisType == 10, true)
		return
	}
	l.alpnACMESnapshot = false
	l.Lfd = listenTcp("0.0.0.0:"+l.Port, true)
}

// needsRebuild 报告该端口是否必须换一个监听器，而不是只热更新配置。
// 两种情况要换：监听类型变了；TLS 端口的 ALPN 通告位变了——
// NextProtos 在建 tls.Config 时就固定，之后改不了。
func (l *Listener) needsRebuild(next *Listener) bool {
	if l.LisType != next.LisType {
		return true
	}
	// 纯 HTTP/TCP 端口的 NextProtos 不受 acme-tls/1 影响，
	// 不能因为 ACME 打开了 tls-alpn-01 就把 80 端口也重建一遍。
	if l.LisType != 1 && l.LisType != 10 {
		return false
	}
	return l.alpnACMESnapshot != TLSALPNEnabled()
}

// updateCfg 热更新监听器配置并发布给请求路径。
// 只允许启动与 reload 协程调用；在途连接通过快照读到完整的新配置。
func (l *Listener) updateCfg(cfg []ListenCfg, hostMap map[string][]ListenCfg) {
	l.Cfg = cfg
	l.HostMap = hostMap
	l.publish()
}

// Hosts 返回请求路由用的 server_name -> locations 映射。
// 手工构造、未经 publish 的 Listener（测试场景）回退读 HostMap 字段。
func (l *Listener) Hosts() map[string][]ListenCfg {
	if s := l.cur.Load(); s != nil {
		return s.HostMap
	}
	return l.HostMap
}

// Cfgs 返回该端口全部 location 配置。回退规则同 Hosts。
func (l *Listener) Cfgs() []ListenCfg {
	if s := l.cur.Load(); s != nil {
		return s.Cfg
	}
	return l.Cfg
}

// DefaultHosts 返回该端口第一个 server 块的 locations，对应 nginx 的
// default_server：Host 不匹配任何 server_name 时用它，否则用 IP 或
// 别名访问会整站 404。Cfg 按配置顺序追加，首项即第一个 server 块。
func (l *Listener) DefaultHosts() []ListenCfg {
	cfgs := l.Cfgs()
	if len(cfgs) == 0 {
		return nil
	}
	return l.Hosts()[cfgs[0].ServerName]
}

// GLisinfos 是当前进程的监听器表。启动与 reload 写入，请求路径与
// 证书热替换（控制面签发证书后）读取，所以读写都走 lisMu：
// 读方拿一份快照，避免在 reload 替换切片时读到半截状态。
var (
	GLisinfos []*Listener
	lisMu     sync.RWMutex
)

// SnapshotListeners 返回当前监听器表的副本。
func SnapshotListeners() []*Listener {
	lisMu.RLock()
	defer lisMu.RUnlock()
	return append([]*Listener(nil), GLisinfos...)
}

func setListeners(list []*Listener) {
	lisMu.Lock()
	GLisinfos = list
	lisMu.Unlock()
}

func FindPorts() []string {
	var Ports []string
	for _, each := range config.Current().Servers {
		arr := strings.Split(each.Listen, " ")
		if !slices.Contains(Ports, arr[0]) {
			Ports = append(Ports, arr[0])
		}
	}
	return Ports
}

func FindOldPorts() []string {
	var Ports []string
	for _, item := range GLisinfos {
		if !slices.Contains(Ports, item.Port) {
			// always true
			Ports = append(Ports, item.Port)
		} else {
			logging.ConsoleLog.Fatal().Msg("find current ports error")
		}
	}
	return Ports
}

// sort confgure form "listen"
// fill port and listen type
func SortByPort(lisInfos *[]*Listener) {
	var Ports []string
	for _, each := range config.Current().Servers {

		arr := strings.Split(each.Listen, " ")
		if !slices.Contains(Ports, arr[0]) {
			Ports = append(Ports, arr[0])

			lis_temp := &Listener{
				HostMap: make(map[string][]ListenCfg),
				Port:    arr[0],
			}
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
			*lisInfos = append(*lisInfos, lis_temp)
		}

	}
}

func SortBySpecificPorts(ports []string, lisInfos *[]*Listener) {
	for _, port := range ports {
		for _, each := range config.Current().Servers { // new config
			if port == strings.Split(each.Listen, " ")[0] {
				lis_temp := &Listener{
					HostMap: make(map[string][]ListenCfg),
					Port:    strings.Split(each.Listen, " ")[0],
				}
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
				*lisInfos = append(*lisInfos, lis_temp)
				break
			}
		}
	}

}

// sort configure from "path"
func processListenData(lisInfos *[]*Listener) {
	Id := 0
	for _, server := range config.Current().Servers {
		listen := strings.Split(server.Listen, " ")[0]
		for _, paths := range server.Path {

			for _, eachlisten := range *lisInfos {
				if eachlisten.Port != listen {
					continue
				}
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
				// 多域名支持：server_name 允许空格分隔多个域名（nginx 风格）。
			// ServerName 保留第一个域名（主域名），其余域名进 Aliases，
			// processEachPort 会把所有域名都注册到 HostMap。
			names := server.ServerNames()
			if len(names) == 0 {
				names = []string{""}
			}
			data.ServerName = names[0] + ":" + eachlisten.Port
			for _, alias := range names[1:] {
				data.Aliases = append(data.Aliases, alias+":"+eachlisten.Port)
			}
			data.Type = paths.PathType
			data.ProxyAddr = paths.ProxyData
			data.ProxySetHeader = paths.ProxySetHeader
			data.AppFireWall = paths.AppFireWall
			data.Limit = paths.Limit
			data.Auth = paths.Auth
			data.StaticRoot = paths.Root
			data.StaticIndex = paths.Index
			data.ErrorNotFound = paths.ErrorNotFound
			data.ErrorProxy = paths.ErrorProxy
			data.SSL = SSLkv{server.SSLCertificate, server.SSLCertificateKey}
			// 多域名：其余域名的证书按 config/cert/<domain>.pem 兜底路径加载，
			// ACME 每个域名分别申请一张证书，certPaths 也按此路径落盘。
			for _, alias := range names[1:] {
				data.ExtraCerts = append(data.ExtraCerts, SSLkv{
					SslKey:   filepath.Join(config.CERT_DIR, alias) + ".pem",
					SslValue: filepath.Join(config.CERT_DIR, alias) + "-key.pem",
				})
			}
			data.Zip = paths.Zip
				data.ReWrite = paths.Rewrite
				data.ProxyCache = paths.ProxyCache
				data.AllowMethods = paths.AllowMethods
				eachlisten.Cfg = append(eachlisten.Cfg, data)
				Id = Id + 1
			}
		}
	}
}

func processHostMap(lisInfos *[]*Listener) {
	for _, eachPort := range *lisInfos {
		processEachPort(eachPort)
	}
}

func processEachPort(lisPort *Listener) {
	var nameArray []string
	for _, item := range lisPort.Cfg {
		if !slices.Contains(nameArray, item.ServerName) {
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
		// 多域名：把该 server 块的所有别名也注册到 HostMap，
		// 指向同一组 locations，hostCfgs 查询时无需感知别名。
		if len(tempListenCfgArray) > 0 {
			for _, alias := range tempListenCfgArray[0].Aliases {
				lisPort.HostMap[alias] = tempListenCfgArray
			}
		}
	}
}

// listen some ports
func ListenWithCfg() []*Listener {
	var CurrLisinfos []*Listener
	SortByPort(&CurrLisinfos)
	processListenData(&CurrLisinfos)
	processHostMap(&CurrLisinfos)

	for _, each := range CurrLisinfos {
		ctx, cancel := context.WithCancel(context.Background())
		each.Ctx = ctx
		each.Cancel = cancel
		each.openListener()
		each.publish()
	}

	setListeners(CurrLisinfos)
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
	logging.PrintInfo("Listen ", laddr)

	if reuse {
		cfg := net.ListenConfig{
			Control: ReuseCallBack,
		}
		listener, err := cfg.Listen(context.Background(), "tcp", laddr)
		if err != nil {
			logging.ConsoleLog.Fatal().Msgf("Error listen: %v", err)
		}
		return listener
	} else {
		listener, err := net.Listen("tcp", laddr)
		if err != nil {
			logging.ConsoleLog.Fatal().Msgf("Error listen: %v", err)
		}
		return listener
	}
}

// ssl listen
//
// h2 为真时才会在 ALPN 中通告 "h2"：不含 h2 的 ssl 端口必须保持纯 HTTP/1.1，
// 否则客户端可能协商到 h2，却被 HTTP/1.1 解析器处理。
func listenSsl(laddr string, certs *certSet, lisdata []ListenCfg, h2 bool, reuse bool) net.Listener {
	if certs == nil {
		certs = newCertSet()
	}
	if err := certs.replace(lisdata); err != nil {
		logging.ConsoleLog.Debug().Msgf("Error load cert: %s", err.Error())
	} else {
		logging.ConsoleLog.Info().Msgf("Automatically load certificate for %s", laddr)
	}

	tlsConfig := &tls.Config{
		GetCertificate: certs.get,
		Time:           time.Now,
		Rand:           rand.Reader,
	}
	if h2 {
		tlsConfig.NextProtos = []string{"h2", "http/1.1"}
	}
	// 只有控制面注册了 TLS-ALPN-01 挑战证书来源时才通告 acme-tls/1，
	// 未启用 ACME 的进程握手表与之前完全一致。
	if TLSALPNEnabled() {
		tlsConfig.NextProtos = append(tlsConfig.NextProtos, ACMEALPNProto)
	}

	tcpListener := listenTcp(laddr, reuse)

	// 在TCP监听器上叠加TLS
	listener := tls.NewListener(tcpListener, tlsConfig)

	return listener
}
