package request

import "strings"

// MaxPathLength 是请求目标（路径 + 查询串）允许的最大长度。
// 超过直接按 400 拒绝：location 正则会作用在整条路径上，超长输入
// 既没有业务意义，也会放大回溯开销。
const MaxPathLength = 8192

// NormalizePath 规范化请求目标，返回规范化后的路径与查询串。
// ok=false 表示应当以 400 拒绝该请求。
//
// 规则（与 nginx 的 merge_slashes + 严格校验对齐）：
//   - 长度不超过 MaxPathLength
//   - 只解码一层 %xx；非法的百分号转义直接拒绝
//   - 解码后仍含 %xx（即二次编码，如 %252f）拒绝
//   - 解码后含反斜杠或控制字符拒绝
//   - 合并连续的 /
//   - 查询串原样保留，不参与规范化
//
// 不以 / 开头的请求目标（绝对形式、*）保持原样，交给调用方按既有逻辑处理。
func NormalizePath(target string) (string, bool) {
	if len(target) > MaxPathLength {
		return "", false
	}
	path, query := target, ""
	if i := strings.IndexByte(target, '?'); i >= 0 {
		path, query = target[:i], target[i:]
	}
	if !strings.HasPrefix(path, "/") {
		return target, true
	}

	decoded, ok := decodePercentOnce(path)
	if !ok {
		return "", false
	}
	// 二次编码：解码后仍能看出 %xx 说明客户端编了两层。
	if hasPercentEscape(decoded) {
		return "", false
	}
	if strings.ContainsRune(decoded, '\\') {
		return "", false
	}
	for i := 0; i < len(decoded); i++ {
		if decoded[i] < 0x20 || decoded[i] == 0x7f {
			return "", false
		}
	}

	merged := mergeSlashes(decoded)
	if merged == "" {
		merged = "/"
	}
	return merged + query, true
}

// decodePercentOnce 解码一层百分号转义。非法转义（% 后面不是两位十六进制）
// 返回 false：RFC 3986 不允许裸 %。
func decodePercentOnce(path string) (string, bool) {
	if !strings.ContainsRune(path, '%') {
		return path, true
	}
	out := make([]byte, 0, len(path))
	for i := 0; i < len(path); i++ {
		if path[i] != '%' {
			out = append(out, path[i])
			continue
		}
		if i+2 >= len(path) {
			return "", false
		}
		high, ok1 := hexValue(path[i+1])
		low, ok2 := hexValue(path[i+2])
		if !ok1 || !ok2 {
			return "", false
		}
		out = append(out, high<<4|low)
		i += 2
	}
	return string(out), true
}

// hasPercentEscape 报告字符串里是否还有 %xx 形态（用于识别二次编码）。
func hasPercentEscape(s string) bool {
	for i := 0; i+2 < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		if _, ok := hexValue(s[i+1]); !ok {
			continue
		}
		if _, ok := hexValue(s[i+2]); ok {
			return true
		}
	}
	return false
}

// mergeSlashes 把连续的 / 合并成一个，保留结尾的单个斜杠。
func mergeSlashes(path string) string {
	if !strings.Contains(path, "//") {
		return path
	}
	out := make([]byte, 0, len(path))
	prevSlash := false
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			if prevSlash {
				continue
			}
			prevSlash = true
			out = append(out, path[i])
			continue
		}
		prevSlash = false
		out = append(out, path[i])
	}
	return string(out)
}

func hexValue(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}
