package logging

import (
	"net"

	"github.com/rs/zerolog"
)

// 拒绝日志（R-10）的统一入口。
//
// 两类级别按影响区分：
//   - RejectWarn：请求被真正拒绝（路径穿越、超限、未知 SNI 等），默认进安全日志；
//   - RejectDebug：请求仍被正常处理，只是按策略跳过某项能力（如压缩跳过），
//     量级与正常请求相同，默认只在排障时打开。

// RejectWarn 返回一条 warn 级别的安全日志事件，已带 reject_reason / rule /
// remote_ip 字段；调用方补 message 或额外字段后 Send/Msg。
func RejectWarn(reason, rule, remoteIP string) *zerolog.Event {
	return rejectEvent(zerolog.WarnLevel, reason, rule, remoteIP)
}

// RejectDebug 返回一条 debug 级别的安全日志事件，字段与 RejectWarn 相同。
func RejectDebug(reason, rule, remoteIP string) *zerolog.Event {
	return rejectEvent(zerolog.DebugLevel, reason, rule, remoteIP)
}

func rejectEvent(level zerolog.Level, reason, rule, remoteIP string) *zerolog.Event {
	ev := Safe().WithLevel(level).Str(FieldRejectReason, reason)
	if rule != "" {
		ev = ev.Str(FieldRule, rule)
	}
	if remoteIP != "" {
		ev = ev.Str(FieldRemoteIP, remoteIP)
	}
	return ev
}

// RemoteIP 返回连接的客户端 IP（去掉端口）。拿不到地址时返回空串，
// 地址不是 host:port 形式时原样返回。
func RemoteIP(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	addr := conn.RemoteAddr()
	if addr == nil {
		return ""
	}
	text := addr.String()
	host, _, err := net.SplitHostPort(text)
	if err != nil {
		return text
	}
	return host
}
