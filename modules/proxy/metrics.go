package proxy

import "sync/atomic"

var (
	dialFailTotal    atomic.Uint64
	sendTimeoutTotal atomic.Uint64
	readTimeoutTotal atomic.Uint64
)

func noteDialFail() {
	dialFailTotal.Add(1)
}

func noteSendTimeout() {
	sendTimeoutTotal.Add(1)
}

func noteReadTimeout() {
	readTimeoutTotal.Add(1)
}

// Stats 是 /runtime 的 upstream 分段。没有代理流量时各字段为 0，那是真实的零。
type Stats struct {
	DialFailTotal    uint64 `json:"dial_fail_total"`
	SendTimeoutTotal uint64 `json:"send_timeout_total"`
	ReadTimeoutTotal uint64 `json:"read_timeout_total"`
	IdleConns        int64  `json:"idle_conns"`
}

// Snapshot 供组合根挂进 /runtime。始终 ok=true。
func Snapshot() (Stats, bool) {
	return Stats{
		DialFailTotal:    dialFailTotal.Load(),
		SendTimeoutTotal: sendTimeoutTotal.Load(),
		ReadTimeoutTotal: readTimeoutTotal.Load(),
		IdleConns:        upstreamPool().idleCount.Load(),
	}, true
}

// ResetMetricsForTest 清空上游失败计数。空闲连接数是池的当前值，不清。
func ResetMetricsForTest() {
	dialFailTotal.Store(0)
	sendTimeoutTotal.Store(0)
	readTimeoutTotal.Store(0)
}
