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
// 此前全进程共用一个 rate.Limiter，一个慢客户端就能把 burst 消耗光，
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
	il := &ipLimiters{
		limiters: make(map[string]*ipLimiter),
		rate:     everyFromRate(config.Current().Limit.Rate),
		burst:    config.Current().Limit.Burst,
	}
	il.resetBucketsLocked()
	g_ips = il
	// 控制面运行时调整过的参数以 overlay 形式存活，不被 reload 冲掉
	applyRateOverlay()
}

// everyFromRate 把"每秒 rate 个请求"换算成 rate.Every 的间隔。
// rate<=0 时返回 Inf（不限流），与旧实现的行为一致。
func everyFromRate(rps int) rate.Limit {
	if rps <= 0 {
		return rate.Inf
	}
	return rate.Every(time.Duration(1000/rps) * time.Millisecond)
}

// setRate 运行时调整限流参数并清空既有桶（新请求按新参数建桶）。
// 调用方负责参数校验与 overlay 记录。
func (il *ipLimiters) setRate(rate, burst int) {
	il.mu.Lock()
	defer il.mu.Unlock()
	il.rate = everyFromRate(rate)
	il.burst = burst
	il.resetBucketsLocked()
}

// resetBucketsLocked 清空全部 IP 桶，调用方必须已持锁。
func (il *ipLimiters) resetBucketsLocked() {
	il.limiters = make(map[string]*ipLimiter)
}

// state 返回当前限流参数与活跃桶数（供控制面查询）。
func (il *ipLimiters) state() RateLimitState {
	il.mu.Lock()
	defer il.mu.Unlock()
	rate := 0
	if il.rate > 0 {
		rate = int(il.rate)
	}
	return RateLimitState{Rate: rate, Burst: il.burst, Buckets: len(il.limiters)}
}

// RateLimitState 是全局 per-IP 限流的状态采样。
type RateLimitState struct {
	Rate    int `json:"rate"`    // 每秒请求数；0 表示不限
	Burst   int `json:"burst"`   // 桶容量
	Buckets int `json:"buckets"` // 活跃 IP 桶数
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
		println("ALLOW-CREATE ip=", ip, " rate=", float64(il.rate), " burst=", il.burst)
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
	stats.totalChecked.Add(1)

	// 检查是否允许进行下一个事件（按客户端 IP 分桶）
	if g_ips.allow(RemoteIP(ev)) {
		return true
	}
	stats.rateLimitRejected.Add(1)

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
