package proxy

import (
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fast-https/config"
)

const maxUpstreamAddrs = 64

// healthWindow 是判断“当前”健康所用的最近一段时间。窗口外的失败只留在累计计数里。
const healthWindow = time.Minute

// healthNow 让测试把失败和恢复放进或移出窗口。
var healthNow = time.Now

var (
	dialFailTotal    atomic.Uint64
	sendTimeoutTotal atomic.Uint64
	readTimeoutTotal atomic.Uint64

	addrMu sync.Mutex
	byAddr = map[string]*addrCounters{}
)

type addrCounters struct {
	dial     atomic.Uint64
	send     atomic.Uint64
	read     atomic.Uint64
	samples  atomic.Uint64
	consec   atomic.Uint64
	lastOK   atomic.Int64
	lastFail atomic.Int64
}

func noteDialFail(addr string) {
	dialFailTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) {
		c.dial.Add(1)
		noteFailure(c)
	})
}

func noteSendTimeout(addr string) {
	sendTimeoutTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) {
		c.send.Add(1)
		noteFailure(c)
	})
}

func noteReadTimeout(addr string) {
	readTimeoutTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) {
		c.read.Add(1)
		noteFailure(c)
	})
}

// noteUpstreamOK 记录一次已经拿到上游响应的请求。拨号或超时失败不走这里。
func noteUpstreamOK(addr string) {
	noteAddr(metricAddr(addr), func(c *addrCounters) {
		c.samples.Add(1)
		c.consec.Store(0)
		c.lastOK.Store(healthNow().UnixNano())
	})
}

func noteFailure(c *addrCounters) {
	c.samples.Add(1)
	c.consec.Add(1)
	c.lastFail.Store(healthNow().UnixNano())
}

func noteAddr(host string, add func(*addrCounters)) {
	if host == "" {
		return
	}
	addrMu.Lock()
	c, ok := byAddr[host]
	if !ok {
		if len(byAddr) >= maxUpstreamAddrs {
			addrMu.Unlock()
			return
		}
		c = &addrCounters{}
		byAddr[host] = c
	}
	addrMu.Unlock()
	add(c)
}

// metricAddr 取出 host:port。去掉协议、路径和用户信息，快照里不出现口令。
func metricAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	raw := addr
	if !strings.Contains(addr, "://") {
		raw = "//" + addr
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// AddrStats 是一个上游地址上的失败计数。
type AddrStats struct {
	Address          string `json:"address"`
	DialFailTotal    uint64 `json:"dial_fail_total"`
	SendTimeoutTotal uint64 `json:"send_timeout_total"`
	ReadTimeoutTotal uint64 `json:"read_timeout_total"`
	Healthy          bool   `json:"healthy"`
	// Health 是当前窗口内的判断：healthy、unhealthy、unknown。
	// Healthy 只在 Health 为 healthy 时为 true。累计失败次数不单独决定它。
	Health  string `json:"health"`
	Drained bool   `json:"drained"`
}

// Stats 是 /runtime 的 upstream 分段。没有代理流量时各字段为 0，那是真实的零。
// 四个总和等于已列出地址之和，再加上超出上限后只进总数的部分。
type Stats struct {
	DialFailTotal    uint64      `json:"dial_fail_total"`
	SendTimeoutTotal uint64      `json:"send_timeout_total"`
	ReadTimeoutTotal uint64      `json:"read_timeout_total"`
	IdleConns        int64       `json:"idle_conns"`
	Upstreams        []AddrStats `json:"upstreams,omitempty"`
}

// Snapshot 供组合根挂进 /runtime。始终 ok=true。
func Snapshot() (Stats, bool) {
	addrMu.Lock()
	counters := make(map[string]*addrCounters, len(byAddr))
	for host, c := range byAddr {
		counters[host] = c
	}
	addrMu.Unlock()

	drained := loadDrained()
	seen := map[string]struct{}{}
	var hosts []string
	addHost := func(host string) {
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		if len(seen) >= maxUpstreamAddrs {
			return
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	for _, host := range configuredMetricAddrs() {
		addHost(host)
	}
	for host := range counters {
		addHost(host)
	}
	sort.Strings(hosts)

	now := healthNow()
	var rows []AddrStats
	if len(hosts) > 0 {
		rows = make([]AddrStats, 0, len(hosts))
		for _, host := range hosts {
			c := counters[host]
			state := healthState(c, now)
			_, isDrained := drained[host]
			row := AddrStats{Address: host, Health: state, Healthy: state == "healthy", Drained: isDrained}
			if c != nil {
				row.DialFailTotal = c.dial.Load()
				row.SendTimeoutTotal = c.send.Load()
				row.ReadTimeoutTotal = c.read.Load()
			}
			rows = append(rows, row)
		}
	}
	return Stats{
		DialFailTotal:    dialFailTotal.Load(),
		SendTimeoutTotal: sendTimeoutTotal.Load(),
		ReadTimeoutTotal: readTimeoutTotal.Load(),
		IdleConns:        upstreamPool().idleCount.Load(),
		Upstreams:        rows,
	}, true
}

func healthState(c *addrCounters, now time.Time) string {
	if c == nil || c.samples.Load() == 0 {
		return "unknown"
	}
	okAt := c.lastOK.Load()
	failAt := c.lastFail.Load()
	okRecent := okAt > 0 && now.Sub(time.Unix(0, okAt)) <= healthWindow
	failRecent := failAt > 0 && now.Sub(time.Unix(0, failAt)) <= healthWindow
	if okRecent && (!failRecent || okAt >= failAt) {
		return "healthy"
	}
	if failRecent && c.consec.Load() > 0 && (!okRecent || failAt > okAt) {
		return "unhealthy"
	}
	return "unknown"
}

func configuredMetricAddrs() []string {
	cfg := config.Current()
	if cfg == nil {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	add := func(addr string) {
		host := metricAddr(addr)
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	for _, server := range cfg.Servers {
		for _, path := range server.Path {
			if len(path.ProxyPeers) > 0 {
				for _, peer := range path.ProxyPeers {
					add(peer)
				}
				continue
			}
			add(path.ProxyData)
		}
	}
	return out
}

// ResetMetricsForTest 清空上游失败计数。空闲连接数是池的当前值，不清。
func ResetMetricsForTest() {
	dialFailTotal.Store(0)
	sendTimeoutTotal.Store(0)
	readTimeoutTotal.Store(0)
	addrMu.Lock()
	byAddr = map[string]*addrCounters{}
	addrMu.Unlock()
}
