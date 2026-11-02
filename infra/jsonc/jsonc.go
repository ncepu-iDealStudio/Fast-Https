// Package jsonc 解析带注释与尾随逗号的 JSON（JSON with comments）。
//
// 配置文件是给人改的，纯 JSON 不允注释就只能把每个键的解释堆到文档里；
// 换 YAML 或 nginx 那种 conf 都要另写一套解析器。这里只加一层预处理：
// 注释与多余逗号原地替换成空格、换行保留，因此 json.SyntaxError.Offset
// 仍然对应原文件的行列，纯 JSON 文件解析结果完全不变。
//
// StripComments 返回与输入等长的字节缓冲，调用方可以继续用原文偏移做
// 结构扫描（config 里的 acme 段就地改写依赖这一点）。
package jsonc

import "encoding/json"

// Unmarshal 解析允许注释与尾随逗号的 JSON。
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(StripComments(data), v)
}

// StripComments 把注释与尾随逗号替换为空格，返回等长缓冲。
// 只处理字符串之外的内容，`"http://x"` 里的 `//` 不会被当成注释。
func StripComments(src []byte) []byte {
	out := make([]byte, len(src))
	copy(out, src)

	inString := false
	escaped := false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '/':
			if i+1 >= len(out) {
				continue
			}
			switch out[i+1] {
			case '/':
				// 行注释：吃到行尾，保留换行
				for i < len(out) && out[i] != '\n' {
					out[i] = ' '
					i++
				}
			case '*':
				// 块注释：吃到 */ 为止，换行原样保留
				end := i + 2
				for end+1 < len(out) && !(out[end] == '*' && out[end+1] == '/') {
					end++
				}
				if end+1 < len(out) {
					end += 2 // 连同 */ 一起吃掉
				}
				for k := i; k < end; k++ {
					if out[k] != '\n' {
						out[k] = ' '
					}
				}
				i = end - 1
			}
		}
	}

	return stripTrailingCommas(out)
}

// stripTrailingCommas 去掉 `}` 或 `]` 前的最后一个逗号。
func stripTrailingCommas(src []byte) []byte {
	inString := false
	escaped := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case ',':
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r' || src[j] == '\n') {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				src[i] = ' '
			}
		}
	}
	return src
}
