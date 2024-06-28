package safe

import (
	"fast-https/config"
	"fast-https/modules/core"
	"fast-https/modules/core/request"
	"fast-https/modules/core/response"
	"fmt"
	"net"
	"strings"
	"sync"
)

// define the blacklist struct
type Blacklist struct {
	ipRanges []string
	mu       sync.RWMutex
}

var g_list Blacklist

// 创建一个新的黑名单
func NewBlacklist() *Blacklist {
	return &Blacklist{
		ipRanges: make([]string, 0),
	}
}

// 获取黑名单中的所有 IP 地址范围
func (b *Blacklist) GetIPRanges() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.ipRanges
}

// the black list add
func (b *Blacklist) Add(ipOrRange string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查重复
	if b.isDuplicate(ipOrRange) {
		return fmt.Errorf("重复的 IP 地址或范围: %s", ipOrRange)
	}

	// 处理逗号分隔的 IP 列表
	ipList, err := parseIPList(ipOrRange)
	if err != nil {
		return err
	}

	// 将每个 IP 添加到黑名单
	for _, ip := range ipList {
		if strings.Contains(ip, "-") {
			// 处理 IP 段
			ipRangeList, err := parseIPRange(ip)
			if err != nil {
				return err
			}

			b.ipRanges = append(b.ipRanges, ipRangeList...)
		} else {
			// 处理单个 IP
			parsedIP := net.ParseIP(ip)
			if parsedIP == nil {
				return fmt.Errorf("无效的 IP 地址: %s", ip)
			}
			b.ipRanges = append(b.ipRanges, parsedIP.String())
		}
	}

	return nil
}

// define the Remove ip function
func (b *Blacklist) Remove(ipOrRange string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查要移除的 IP 是否存在
	if !b.isInBlacklist(ipOrRange) {
		return fmt.Errorf("IP 或 IP 段 %s 不在黑名单中", ipOrRange)
	}

	if strings.Contains(ipOrRange, "-") {
		// 处理 IP 段
		ipList, err := parseIPRange(ipOrRange)
		if err != nil {
			return err
		}

		for _, ip := range ipList {
			for i, existingIP := range b.ipRanges {
				if existingIP == ip {
					// 找到匹配的 IP，移除
					b.ipRanges = append(b.ipRanges[:i], b.ipRanges[i+1:]...)
				}
			}
		}
	} else {
		// 处理单个 IP
		parsedIP := net.ParseIP(ipOrRange)
		if parsedIP == nil {
			return fmt.Errorf("无效的 IP 地址: %s", ipOrRange)
		}

		for i, existingIP := range b.ipRanges {
			if existingIP == parsedIP.String() {
				// 找到匹配的 IP，移除
				b.ipRanges = append(b.ipRanges[:i], b.ipRanges[i+1:]...)
				return nil
			}
		}
	}

	return fmt.Errorf("IP 或 IP 段 %s 不在黑名单中", ipOrRange)
}

// 检查是否存在重复的 IP 地址或范围
func (b *Blacklist) isDuplicate(ipOrRange string) bool {
	for _, existingIP := range b.ipRanges {
		if existingIP == ipOrRange {
			return true
		}
	}
	return false
}

// 检查 IP 或 IP 段是否在黑名单中
func (b *Blacklist) isInBlacklist(ipOrRange string) bool {
	for _, existingIP := range b.ipRanges {
		if existingIP == ipOrRange {
			return true
		}
	}
	return false
}

// api
func IsInBlacklist(ev *core.Event) bool {
	// fmt.Println(strings.Split(ev.Conn.RemoteAddr().String(), ":")[0])
	if g_list.isInBlacklist(strings.Split(ev.Conn.RemoteAddr().String(), ":")[0]) {

		ev.RR.Res = response.ResponseInit()
		ev.RR.Req = request.RequestInit(false)
		useless_data := make([]byte, 2048)
		ev.Conn.Read(useless_data)
		res := response.DefaultBlackBan()
		ev.RR.Res = res
		ev.WriteResponseClose(nil)
		core.Log(&ev.Log, ev, "")
		return true
	} else {
		return false
	}
}

// 解析 IP 列表并返回 IP 地址列表
func parseIPList(ipListStr string) ([]string, error) {
	ipList := strings.Split(ipListStr, ",")

	var result []string
	for _, ipStr := range ipList {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr != "" {
			result = append(result, ipStr)
		}
	}

	return result, nil
}

// 解析 IP 段并返回 IP 地址列表
func parseIPRange(ipRange string) ([]string, error) {
	parts := strings.Split(ipRange, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("无效的 IP 段格式")
	}

	startIP := net.ParseIP(strings.TrimSpace(parts[0]))
	endIP := net.ParseIP(strings.TrimSpace(parts[1]))

	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("无效的 IP 地址")
	}

	var ips []string
	for ip := startIP; ip.String() != endIP.String(); {
		ips = append(ips, ip.String())
		ip = nextIP(ip)
	}
	ips = append(ips, endIP.String())

	return ips, nil
}

// 计算下一个 IP 地址
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}

	return next
}

func blacklistInit() {
	g_list = *NewBlacklist()
	for _, value := range config.GConfig.BlackList {
		g_list.Add(value)
	}

	// fmt.Println(g_list.isInBlacklist("127.0.0.1"))
}
