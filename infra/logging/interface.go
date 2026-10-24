package logging

import (
	"fmt"

	"github.com/rs/zerolog"
)

// PrintInfo 向系统日志写一条提示。
func PrintInfo(a ...interface{}) {
	SystemLog.Info().Msg(fmt.Sprint(a...))
}

// PrintWarn 向系统日志写一条告警。
func PrintWarn(a ...interface{}) {
	SystemLog.Warn().Msg(fmt.Sprint(a...))
}

// PrintErr 向错误日志写一条错误。
func PrintErr(a ...interface{}) {
	ErrorLog.Error().Msg(fmt.Sprint(a...))
}

// AccessEvent 返回一条待填充的访问日志事件。
//
// 调用方直接往事件上加字段再 Send，避免为每条请求分配中间结构。
func AccessEvent() *zerolog.Event {
	return AccessLog.Info()
}
