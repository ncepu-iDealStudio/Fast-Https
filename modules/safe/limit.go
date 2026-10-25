package safe

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/response"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// limiterIdleTTL 是 per-IP 限流器的空闲回收阈值：超过它没有请求的
// IP 条目在周期清扫时删除，map 不随历史客户端无界增长。
const limiterIdleTTL = 5 * time.Minute

// ipLimiters 为每个客户端 IP 维护独立的令牌桶。
//
// 此前全进程共用一个 rate.Limiter，一个慢客户端就能把 burst 消耗光、
// 拒掉所有人；按 IP 分桶后各客户端互不影响。
type ipLimiters struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	rate     rate.Limit
	burst    int
}

type ipLimiter struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

var g_ips *ipLimiters

func limitInit() {
	// 保留既有换算：每秒 rate 个请求
	temp := float64(1.00 / float64(config.Current().Limit.Rate) * 1000)
	g_ips = &ipLimiters{
		limiters: make(map[string]*ipLimiter),
		rate:     rate.Every(time.Duration(int(temp)) * time.Millisecond),
		burst:    config.Current().Limit.Burst,
	}
}

// allow 报告该 IP 是否还能立即获得一个令牌。
func (il *ipLimiters) allow(ip string) bool {
	il.mu.Lock()
	defer il.mu.Unlock()

	now := time.Now()
	il.sweepLocked(now)
	l, ok := il.limiters[ip]
	if !ok {
		l = &ipLimiter{lim: rate.NewLimiter(il.rate, il.burst)}
		il.limiters[ip] = l
	}
	l.lastSeen = now
	return l.lim.Allow()
}

// sweepLocked 删除长时间不活跃的 IP 桶，调用方必须已持锁。
func (il *ipLimiters) sweepLocked(now time.Time) {
	for ip, l := range il.limiters {
		if now.Sub(l.lastSeen) >= limiterIdleTTL {
			delete(il.limiters, ip)
		}
	}
}

func Bucket(ev *core.Event) bool {

	// 检查是否允许进行下一个事件（按客户端 IP 分桶）
	if g_ips.allow(RemoteIP(ev)) {
		return true
	}

	// write <403> and close
	logReject(ev, ReasonRateLimit)
	log := dynlog.DynLogger{}
	dynlog.Log(&log, ev)

	// HTTP/2 下底层连接由协议栈持有，直接 Read 会吞掉帧数据。
	if ev.RR.Req == nil || !ev.RR.Req.H2 {
		buffer := make([]byte, 1024)
		ev.Conn.Read(buffer)
	}
	ev.RR.Res = response.DefaultTooMany()
	ev.WriteResponseClose(nil)
	return false
}
