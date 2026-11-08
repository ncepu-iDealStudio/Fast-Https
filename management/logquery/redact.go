package logquery

import (
	"regexp"
	"strings"

	"fast-https/management/redact"
)

// 日志行里的 path、message 是不可信文本。这里只改写出的副本，
// 过滤（路径前缀、时间）仍用原文，避免脱敏把本该命中的记录滤掉。

var (
	queryPattern  = regexp.MustCompile(`\?[^\s#]*`)
	bearerPattern = regexp.MustCompile(`(?i)((?:authorization:\s*)?bearer\s+)\S+`)
	basicPattern  = regexp.MustCompile(`(?i)((?:authorization:\s*)?basic\s+)[A-Za-z0-9+/=]+`)
	pemPattern    = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
)

// redactText 遮蔽查询串里的敏感参数、Bearer / Basic 凭据和私钥正文。
// 其它文字原样保留：日志内容是数据，不能因为出现某些词就被删掉或拿去执行。
func redactText(s string) string {
	if s == "" {
		return s
	}
	s = queryPattern.ReplaceAllStringFunc(s, redactQuery)
	s = bearerPattern.ReplaceAllString(s, "${1}"+redact.Mask)
	s = basicPattern.ReplaceAllString(s, "${1}"+redact.Mask)
	s = pemPattern.ReplaceAllString(s, redact.Mask)
	return s
}

func redactQuery(q string) string {
	raw := strings.TrimPrefix(q, "?")
	parts := strings.Split(raw, "&")
	for i, p := range parts {
		key, val, ok := strings.Cut(p, "=")
		if !ok || val == "" {
			continue
		}
		if redact.IsSensitiveKey(key) {
			parts[i] = key + "=" + redact.Mask
		}
	}
	return "?" + strings.Join(parts, "&")
}
