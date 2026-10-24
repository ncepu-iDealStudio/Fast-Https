// Package safe 实现限流、临时黑名单与连接计数。
package safe

import (
	"fast-https/core"
	"fast-https/infra/logging"
	"strings"
)

// 安全日志里的 reason 取值。
const (
	ReasonRateLimit     = "rate_limit"
	ReasonLocationLimit = "location_limit"
	ReasonBlacklist     = "blacklist"
)

// Init 按当前配置重建限流、计数与黑名单状态。
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
