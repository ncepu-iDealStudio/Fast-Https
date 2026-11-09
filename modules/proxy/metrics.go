package proxy

import (
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

const maxUpstreamAddrs = 64

var (
	dialFailTotal    atomic.Uint64
	sendTimeoutTotal atomic.Uint64
	readTimeoutTotal atomic.Uint64

	addrMu sync.Mutex
	byAddr = map[string]*addrCounters{}
)

type addrCounters struct {
	dial atomic.Uint64
	send atomic.Uint64
	read atomic.Uint64
}

func noteDialFail(addr string) {
	dialFailTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) { c.dial.Add(1) })
}

func noteSendTimeout(addr string) {
	sendTimeoutTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) { c.send.Add(1) })
}

func noteReadTimeout(addr string) {
	readTimeoutTotal.Add(1)
	noteAddr(metricAddr(addr), func(c *addrCounters) { c.read.Add(1) })
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
	hosts := make([]string, 0, len(byAddr))
	counters := make([]*addrCounters, 0, len(byAddr))
	for host, c := range byAddr {
		hosts = append(hosts, host)
		counters = append(counters, c)
	}
	addrMu.Unlock()

	var rows []AddrStats
	if len(hosts) > 0 {
		order := make([]int, len(hosts))
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(i, j int) bool { return hosts[order[i]] < hosts[order[j]] })
		rows = make([]AddrStats, 0, len(order))
		for _, i := range order {
			rows = append(rows, AddrStats{
				Address:          hosts[i],
				DialFailTotal:    counters[i].dial.Load(),
				SendTimeoutTotal: counters[i].send.Load(),
				ReadTimeoutTotal: counters[i].read.Load(),
			})
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

// ResetMetricsForTest 清空上游失败计数。空闲连接数是池的当前值，不清。
func ResetMetricsForTest() {
	dialFailTotal.Store(0)
	sendTimeoutTotal.Store(0)
	readTimeoutTotal.Store(0)
	addrMu.Lock()
	byAddr = map[string]*addrCounters{}
	addrMu.Unlock()
}
