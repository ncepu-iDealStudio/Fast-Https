package core

import (
	"fast-https/config"
	"fast-https/utils/message"
)

var (
	GLogMap []func(*Logger, string, *Event)
	split   string
)

type Logger struct {
	s      string
	status string
	size   string
}

func LogRegister() {
	if config.GConfig.LogSplit == "" {
		config.GConfig.LogSplit = " "
	}
	split = config.GConfig.LogSplit
	if config.GConfig.LogFormat == nil {
		config.GConfig.LogFormat = []string{"ip_port", "time", "type", "method", "path", "host", "status", "size", "user_agent"}
	}
	for _, log := range config.GConfig.LogFormat {
		switch log {
		case "ip_port":
			GLogMap = append(GLogMap, log_ip_port)
		case "time":
			GLogMap = append(GLogMap, log_time)
		case "type":
			GLogMap = append(GLogMap, log_type)
		case "method":
			GLogMap = append(GLogMap, log_method)
		case "path":
			GLogMap = append(GLogMap, log_path)
		case "host":
			GLogMap = append(GLogMap, log_host)
		case "status":
			GLogMap = append(GLogMap, log_status)
		case "size":
			GLogMap = append(GLogMap, log_size)
		case "user_agent":
			GLogMap = append(GLogMap, log_user_agent)
		}
	}
}

func NewLogger() *Logger {
	return &Logger{}
}

func Log(l *Logger, ev *Event, s string) {
	for _, log := range GLogMap {
		log(l, s, ev)
	}
	str := l.s
	message.PrintAccess(str)
}

// append string after index
func LogOther(l *Logger, k string, v string) {
	switch k {
	case "status":
		l.status = v
	case "size":
		l.size = v
	}
}

func LogClear(l *Logger) {
	l.s = ""
}

func log_ip_port(l *Logger, s string, ev *Event) {
	l.s += "\"" + ev.Conn.RemoteAddr().String() + "\""
	l.s += split
}

func log_time(l *Logger, s string, ev *Event) {

}

func log_type(l *Logger, s string, ev *Event) {
	switch ev.Type {
	case config.LOCAL:
		l.s += "\"" + "LOCAL" + "\""
	case config.PROXY_HTTP:
		l.s += "\"" + "PROXY_HTTP" + "\""
	case config.PROXY_HTTPS:
		l.s += "\"" + "PROXY_HTTPS" + "\""
	case config.PROXY_TCP:
		l.s += "\"" + "PROXY_TCP" + "\""
	case config.REWRITE:
		l.s += "\"" + "REWRITE" + "\""
	}
	l.s += split
}

func log_method(l *Logger, s string, ev *Event) {
	l.s += "\"" + ev.RR.Req.Method + "\""
	l.s += split
}

func log_path(l *Logger, s string, ev *Event) {
	l.s += "\"" + ev.RR.Req.Path + "\""
	l.s += split
}

func log_host(l *Logger, s string, ev *Event) {
	l.s += "\"" + ev.RR.Req.GetHost() + "\""
	l.s += split
}

func log_status(l *Logger, s string, ev *Event) {
	l.s += "\"" + l.status + "\""
	l.s += split
}

func log_size(l *Logger, s string, ev *Event) {
	l.s += "\"" + l.size + "\""
	l.s += split
}

func log_user_agent(l *Logger, s string, ev *Event) {
	l.s += "\"" + ev.RR.Req.Headers["User-Agent"] + "\""
	l.s += split
}
