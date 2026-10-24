// Package dynlog 按 http.log_format 组装访问日志的结构化字段。
package dynlog

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"

	"github.com/rs/zerolog"
)

// DynLogger 暂存一条访问日志中由处理过程决定的字段。
//
// 零值可用：只有处理流程真正产生状态码或长度时才需要赋值。
type DynLogger struct {
	status int
	size   int
}

// Status 记录响应状态码。
func (l *DynLogger) Status(code int) {
	l.status = code
}

// Size 记录响应体字节数。
func (l *DynLogger) Size(n int) {
	l.size = n
}

type fieldFunc func(*zerolog.Event, *DynLogger, *core.Event)

// GLogMap 是按配置顺序排好的字段写入函数。
var GLogMap []fieldFunc

// fieldFuncs 是配置名到字段写入函数的映射，替代逐项 switch。
var fieldFuncs = map[string]fieldFunc{
	"ip_port":    fieldRemoteAddr,
	"time":       fieldTime,
	"type":       fieldType,
	"method":     fieldMethod,
	"path":       fieldPath,
	"host":       fieldHost,
	"status":     fieldStatus,
	"size":       fieldSize,
	"user_agent": fieldUserAgent,
}

// LogRegister 按 http.log_format 重建字段函数表。
// 重复调用（例如 reload）先清空，避免字段被重复追加。
func LogRegister() {
	if config.Current().LogFormat == nil {
		config.Current().LogFormat = []string{"ip_port", "time", "type", "method", "path", "host", "status", "size", "user_agent"}
	}
	GLogMap = GLogMap[:0]
	for _, name := range config.Current().LogFormat {
		if fn, ok := fieldFuncs[name]; ok {
			GLogMap = append(GLogMap, fn)
		}
	}
}

// Log 写出这条访问日志。
//
// 直接往 zerolog 事件上加字段，不为每条请求分配中间结构。
func Log(l *DynLogger, ev *core.Event) {
	event := logging.AccessEvent()
	for _, fn := range GLogMap {
		fn(event, l, ev)
	}
	event.Send()
}

func fieldRemoteAddr(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldRemoteAddr, ev.Conn.RemoteAddr().String())
}

func fieldTime(_ *zerolog.Event, _ *DynLogger, _ *core.Event) {
	// 时间戳由 zerolog 统一写入，这里保持配置项兼容但不重复输出。
}

func fieldType(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldType, pathTypeName(ev.Type))
}

func fieldMethod(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldMethod, ev.RR.Req.Method)
}

func fieldPath(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldPath, ev.RR.Req.Path)
}

func fieldHost(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldHost, ev.RR.Req.GetHost())
}

func fieldStatus(e *zerolog.Event, l *DynLogger, _ *core.Event) {
	e.Int(logging.FieldStatus, l.status)
}

func fieldSize(e *zerolog.Event, l *DynLogger, _ *core.Event) {
	e.Int(logging.FieldSize, l.size)
}

func fieldUserAgent(e *zerolog.Event, _ *DynLogger, ev *core.Event) {
	e.Str(logging.FieldUserAgent, ev.RR.Req.Headers["User-Agent"])
}

func pathTypeName(t uint16) string {
	switch t {
	case config.LOCAL:
		return "LOCAL"
	case config.PROXY_HTTP:
		return "PROXY_HTTP"
	case config.PROXY_HTTPS:
		return "PROXY_HTTPS"
	case config.PROXY_TCP:
		return "PROXY_TCP"
	case config.REWRITE:
		return "REWRITE"
	default:
		return ""
	}
}
