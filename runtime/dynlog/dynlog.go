// Package dynlog 按 http.log_format 组装访问日志的结构化字段。
package dynlog

import (
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"sync/atomic"

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

// defaultLogFormat 是 http.log_format 未配置时的默认字段序。
var defaultLogFormat = []string{"ip_port", "time", "type", "method", "path", "host", "status", "size", "user_agent"}

// logFields 是请求路径读取的字段函数表，atomic 整体替换：
// reload 重建时请求协程要么读到完整的旧表、要么读到完整的新表。
var logFields atomic.Pointer[[]fieldFunc]

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
// 重复调用（例如 reload）整体替换，不会重复追加字段。
func LogRegister() {
	format := config.Current().LogFormat
	if format == nil {
		format = defaultLogFormat
	}
	fields := make([]fieldFunc, 0, len(format))
	for _, name := range format {
		if fn, ok := fieldFuncs[name]; ok {
			fields = append(fields, fn)
		}
	}
	logFields.Store(&fields)
}

// Log 写出这条访问日志。
//
// 直接往 zerolog 事件上加字段，不为每条请求分配中间结构。
func Log(l *DynLogger, ev *core.Event) {
	fields := logFields.Load()
	if fields == nil {
		// 防御：LogRegister 未跑过（如早期拒绝路径或测试环境）时
		// 惰性构建默认字段表；并发重复构建是幂等的
		LogRegister()
		fields = logFields.Load()
	}
	event := logging.AccessEvent()
	for _, fn := range *fields {
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
