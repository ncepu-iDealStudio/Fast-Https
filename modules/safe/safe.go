// Package safe 实现限流、临时黑名单与连接计数，
// 并提供控制面（agent/管理端）的运行时管理接口与状态收集。
package safe

import (
	"strings"
	"sync/atomic"

	"fast-https/core"
	"fast-https/infra/logging"
)

// 安全日志里的 reason 取值。
const (
	ReasonRateLimit     = "rate_limit"
	ReasonLocationLimit = "location_limit"
	ReasonBlacklist     = "blacklist"
	ReasonControl       = "control"
)

// 数据面状态计数：控制面（agent/管理端）经 Snapshot 查询。
var stats struct {
	totalChecked      atomic.Int64
	blacklistRejected atomic.Int64
	rateLimitRejected atomic.Int64
	locationRejected  atomic.Int64
}

// StatsSnapshot 是数据面状态的一次采样，字段可 JSON 序列化，
// 供控制面直接回传。
type StatsSnapshot struct {
	TotalChecked      int64            `json:"total_checked"`
	BlacklistRejected int64            `json:"blacklist_rejected"`
	RateLimitRejected int64            `json:"rate_limit_rejected"`
	LocationRejected  int64            `json:"location_rejected"`
	BlacklistSize     int              `json:"blacklist_size"`
	ActiveIPBuckets   int              `json:"active_ip_buckets"`
	GlobalRate        int              `json:"global_rate"`
	GlobalBurst       int              `json:"global_burst"`
	Blacklist         []BlacklistEntry `json:"blacklist"`
}

// Snapshot 采样当前数据面状态。
func Snapshot() StatsSnapshot {
	rl := GetGlobalRateLimit()
	return StatsSnapshot{
		TotalChecked:      stats.totalChecked.Load(),
		BlacklistRejected: stats.blacklistRejected.Load(),
		RateLimitRejected: stats.rateLimitRejected.Load(),
		LocationRejected:  stats.locationRejected.Load(),
		BlacklistSize:     g_list.Size(),
		ActiveIPBuckets:   rl.Buckets,
		GlobalRate:        rl.Rate,
		GlobalBurst:       rl.Burst,
		Blacklist:         ListBlacklist(),
	}
}

// Init 按当前配置重建限流、计数与黑名单状态。
// 控制面的运行时变更以 overlay 形式在重建后重新应用。
func Init() {
	countsInit()
	limitInit()
	blacklistInit()
}

// RemoteIP 去掉端口，只保留客户端 IP。
func RemoteIP(ev *core.Event) string {
	addr := ev.Conn.RemoteAddr().String()
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

// logReject 向安全日志写一条拒绝事件，字段与访问日志保持一致，便于统一查询。
func logReject(ev *core.Event, reason string) {
	logging.Safe().Warn().
		Str(logging.FieldRemoteAddr, RemoteIP(ev)).
		Str(logging.FieldReason, reason).
		Msg("connection rejected")
}
