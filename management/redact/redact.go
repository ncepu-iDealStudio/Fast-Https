// Package redact 在配置、站点与日志的视图里遮蔽敏感值。
//
// 规则按键名判断，不按值判断：凭据没有可靠的格式，键名才是稳定信号。
// 路径（例如 ssl_certificate_key 指向的文件）不算秘密，原样保留；
// 私钥正文不会出现在配置里，不在这里处理。
package redact

import "strings"

// Mask 是被遮蔽的值。
const Mask = "***"

var exactKeys = map[string]struct{}{
	"pswd":         {},
	"passwd":       {},
	"password":     {},
	"pwd":          {},
	"token":        {},
	"secret":       {},
	"eab_kid":      {},
	"eab_hmac_key": {},
	"hmac_key":     {},
	"api_key":      {},
	"apikey":       {},
	"private_key":  {},
	"access_key":   {},
}

// 键名里含这些片段就当成敏感（access_token、client_secret、db_password……）。
var keyFragments = []string{"password", "passwd", "secret", "token"}

// 作为请求头转发给上游时需要遮蔽值的头名。
var sensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"x-auth-token":        {},
}

// IsSensitiveKey 报告键名是否指向敏感值。
func IsSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if _, ok := exactKeys[k]; ok {
		return true
	}
	for _, f := range keyFragments {
		if strings.Contains(k, f) {
			return true
		}
	}
	return false
}

// Value 返回遮蔽后的副本，输入保持不变。支持 encoding/json 解出来的
// map[string]any 与 []any；其它类型原样返回。
func Value(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return redactMap(x)
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = Value(x[i])
		}
		return out
	default:
		return v
	}
}

func redactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) && !isContainer(v) {
			out[k] = maskScalar(v)
			continue
		}
		out[k] = Value(v)
	}
	// proxy_set_header 的元素是 {HeaderKey, HeaderValue} 成对出现，
	// 敏感与否取决于头名，而不是 HeaderValue 这个键。
	if name, ok := lookupString(out, "HeaderKey"); ok {
		if _, sensitive := sensitiveHeaders[strings.ToLower(strings.TrimSpace(name))]; sensitive {
			for k := range out {
				if strings.EqualFold(k, "HeaderValue") {
					out[k] = Mask
				}
			}
		}
	}
	return out
}

func lookupString(m map[string]any, key string) (string, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			s, ok := v.(string)
			return s, ok
		}
	}
	return "", false
}

func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

// maskScalar 保留「有没有设置」这一信息：空串与 null 原样返回，
// 否则排查「口令是不是漏配了」时会被遮蔽误导。
func maskScalar(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if x == "" {
			return x
		}
	}
	return Mask
}
