package safe

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"

	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// BlacklistEntry 是黑名单的一条录入。
type BlacklistEntry struct {
	Raw    string   `json:"raw"`    // 原始录入形态（单 IP 或 "a-b" 段）
	IPs    []string `json:"ips"`    // 展开后的 IP 列表
	Source string   `json:"source"` // config（配置加载）| runtime（控制面添加）
}

// Blacklist 是 IP 黑名单：按录入（单 IP 或段）管理，命中判断基于展开后的 IP。
// 所有读写都持锁；录入来源区分 config/runtime，供控制面查询与 overlay 恢复。
type Blacklist struct {
	mu      sync.RWMutex
	entries []BlacklistEntry
}

func NewBlacklist() *Blacklist {
	return &Blacklist{}
}

// Add 添加一条录入（单 IP、"a,b" 列表或 "a-b" 段）。
// source 取 BlacklistSourceConfig 或 BlacklistSourceRuntime；重复录入返回错误。
func (b *Blacklist) Add(source, entry string) error {
	ips, err := expandEntry(entry)
	if err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.entries {
		if e.Raw == entry {
			return fmt.Errorf("重复的 IP 地址或范围: %s", entry)
		}
	}
	b.entries = append(b.entries, BlacklistEntry{Raw: entry, IPs: ips, Source: source})
	return nil
}

// Remove 移除一条录入：按原始形态匹配，也接受展开后的单个 IP
// （命中某段的 IP 时整段移除）。
func (b *Blacklist) Remove(entry string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, e := range b.entries {
		if e.Raw == entry {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			return nil
		}
		for _, ip := range e.IPs {
			if ip == entry {
				b.entries = append(b.entries[:i], b.entries[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("IP 或 IP 段 %s 不在黑名单中", entry)
}

// Contains 报告 ip 是否命中黑名单（按展开后的 IP 线性比对）。
func (b *Blacklist) Contains(ip string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, e := range b.entries {
		for _, listed := range e.IPs {
			if listed == ip {
				return true
			}
		}
	}
	return false
}

// Entries 返回录入的拷贝（供控制面查询）。
func (b *Blacklist) Entries() []BlacklistEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]BlacklistEntry, len(b.entries))
	copy(out, b.entries)
	return out
}

// Size 返回录入条数。
func (b *Blacklist) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.entries)
}

// g_list 是进程唯一的黑名单实例（连接检查与控制面共用）。
var g_list Blacklist

// blacklistInit 以配置为底、叠加控制面 overlay 重建黑名单。
// reload 会调用：控制面的运行时变更以 overlay 形式存活，不被冲掉。
func blacklistInit() {
	g_list = *NewBlacklist()
	for _, value := range config.Current().BlackList {
		if err := g_list.Add(BlacklistSourceConfig, value); err != nil {
			logging.ConsoleLog.Warn().Msgf("blacklist: skip invalid config entry %q: %s", value, err.Error())
		}
	}
	applyBlacklistOverlay()
}

// IsInBlacklist 连接级黑名单检查（cmd 的 RejectConn 钩子调用）。
// 命中时写 403 拒绝响应与安全日志，返回 true。
func IsInBlacklist(ev *core.Event) bool {
	// IPv6 地址形如 "[::1]:8080"，必须用 SplitHostPort 取主机部分
	host, _, err := net.SplitHostPort(ev.Conn.RemoteAddr().String())
	if err != nil {
		host = ev.Conn.RemoteAddr().String()
	}
	if !g_list.Contains(host) {
		return false
	}
	stats.blacklistRejected.Add(1)

	ev.RR.Res = response.ResponseInit()
	ev.RR.Req = request.RequestInit(false)
	drain := make([]byte, 2048)
	_, _ = ev.Conn.Read(drain)
	ev.RR.Res = response.DefaultBlackBan()
	log := dynlog.DynLogger{}
	dynlog.Log(&log, ev)
	logReject(ev, ReasonBlacklist)
	ev.WriteResponseClose(nil)
	return true
}

// 解析逗号分隔的 IP 列表
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

// 解析 "a-b" IP 段并展开为 IP 地址列表（含上限防护）
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
	if bytes.Compare(startIP.To16(), endIP.To16()) > 0 {
		return nil, fmt.Errorf("IP 段起始地址大于结束地址")
	}

	// 旧实现以字符串相等作为循环终止条件：起止反序时 nextIP 永远追不上
	// 终点，会死循环；这里改为 Equal 判定并用上限兜底任何异常配置。
	var ips []string
	for ip := startIP; ; ip = nextIP(ip) {
		ips = append(ips, ip.String())
		if ip.Equal(endIP) {
			break
		}
		if len(ips) > maxIPRangeSize {
			return nil, fmt.Errorf("IP 段过大（超过 %d 个地址），拒绝展开", maxIPRangeSize)
		}
	}

	return ips, nil
}

// maxIPRangeSize 限制一个 IP 段展开后的地址个数，
// 防止 "0.0.0.0-255.255.255.255" 这类配置展开成数十亿个字符串。
const maxIPRangeSize = 1 << 16

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

// expandEntry 把录入展开成 IP 列表：支持逗号分隔列表与 "a-b" 段。
func expandEntry(entry string) ([]string, error) {
	ips, err := parseIPList(entry)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, item := range ips {
		if strings.Contains(item, "-") {
			ranged, err := parseIPRange(item)
			if err != nil {
				return nil, err
			}
			out = append(out, ranged...)
			continue
		}
		parsed := net.ParseIP(item)
		if parsed == nil {
			return nil, fmt.Errorf("无效的 IP 地址: %s", item)
		}
		out = append(out, parsed.String())
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("空的 IP 录入")
	}
	return out, nil
}
