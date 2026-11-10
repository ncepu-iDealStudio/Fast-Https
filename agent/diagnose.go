package main

import "strings"

// Diagnosis 是规则诊断结果。原因引用事件原文，不调用模型。
type Diagnosis struct {
	Classification string
	Subject        string
	Cause          string
	Impact         string
	Evidence       []string
	NeedsHuman     bool
}

func diagnose(monitorType, message, startedAt, contextJSON string) Diagnosis {
	class := classify(monitorType)
	subject := "server"
	if startedAt != "" {
		subject = "server started_at=" + startedAt
	}
	cause := causeText(class, message, contextJSON)
	needs := class != "service_restarted" && class != "recovered"
	return Diagnosis{
		Classification: class,
		Subject:        subject,
		Cause:          cause,
		Impact:         impactText(class),
		Evidence:       []string{"event.message", "runtime"},
		NeedsHuman:     needs,
	}
}

func classify(monitorType string) string {
	switch monitorType {
	case "unreachable":
		return "service_unavailable"
	case "recovered":
		return "recovered"
	case "restarted":
		return "service_restarted"
	case "5xx_ratio":
		return "http_5xx_spike"
	case "port_probe":
		return "port_unreachable"
	case "stuck_suspected":
		return "stuck_suspected"
	case "collect_error":
		return "collect_error"
	case "certificate_expiring":
		return "certificate_expiring"
	case "certificate_read_error":
		return "certificate_read_error"
	default:
		return "unknown"
	}
}

func causeText(class, message, contextJSON string) string {
	base := map[string]string{
		"service_unavailable":    "服务快照连续采集失败。",
		"http_5xx_spike":         "最近窗口内 5xx 比例升高。",
		"port_unreachable":       "运行接口正常，但业务端口探测失败。",
		"stuck_suspected":        "活动请求长时间不降，且累计请求不再增长。",
		"service_restarted":      "进程 pid 或启动时间发生变化。",
		"collect_error":          "采集失败类别发生变化。",
		"certificate_expiring":   "证书剩余天数低于阈值。",
		"certificate_read_error": "证书文件无法读取。",
		"recovered":              "采集已恢复。",
		"unknown":                "事件类型没有对应的规则。",
	}
	text := base[class]
	if text == "" {
		text = base["unknown"]
	}
	text += "事件说明：" + message
	if n, ok := jsonNumber(contextJSON, "responses_502_total"); ok {
		text += "。runtime.responses_502_total=" + n
	}
	if n, ok := jsonNumber(contextJSON, "responses_504_total"); ok {
		text += "。runtime.responses_504_total=" + n
	}
	return text
}

func impactText(class string) string {
	switch class {
	case "service_unavailable", "port_unreachable":
		return "业务端口可能不可用"
	case "http_5xx_spike":
		return "部分请求返回 5xx"
	case "certificate_expiring":
		return "证书到期后 TLS 握手会失败"
	default:
		return "需要人工查看，本阶段不自动修改配置"
	}
}

func jsonNumber(raw, key string) (string, bool) {
	i := strings.Index(raw, `"`+key+`"`)
	if i < 0 {
		return "", false
	}
	rest := raw[i+len(key)+2:]
	rest = strings.TrimLeft(rest, " \t:")
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return "", false
	}
	return rest[:end], true
}
