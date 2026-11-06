package jsonc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Member 是某个对象成员在原文中的位置。偏移都以 StripComments 的掩码为准，
// 掩码与原文等长，因此偏移可以直接切原文。
type Member struct {
	Key        string
	KeyStart   int
	ValueStart int
	ValueEnd   int
}

// Object 是根对象的结构：左右花括号位置与全部顶层成员。
type Object struct {
	Open    int
	Close   int
	Members []Member
}

// ScanObject 扫描根对象。masked 应为 StripComments 的结果：
// 注释已经是空格，字符串之外的标点才认得准。
func ScanObject(masked []byte) (Object, error) {
	pos := skipSpace(masked, 0)
	if pos >= len(masked) || masked[pos] != '{' {
		return Object{}, fmt.Errorf("root must be a JSON object")
	}
	return scanObjectAt(masked, pos)
}

// scanObjectAt 从 open（必须指向 '{'）开始扫描一个对象，
// 成员偏移量都是相对整个 masked 的绝对位置。
func scanObjectAt(masked []byte, open int) (Object, error) {
	if open >= len(masked) || masked[open] != '{' {
		return Object{}, fmt.Errorf("expected object at offset %d", open)
	}
	pos := skipSpace(masked, open+1)
	var members []Member

	for {
		if pos >= len(masked) {
			return Object{}, fmt.Errorf("unterminated object at offset %d", open)
		}
		switch masked[pos] {
		case '}':
			return Object{Open: open, Close: pos, Members: members}, nil
		case ',':
			pos = skipSpace(masked, pos+1)
			continue
		}

		keyStart := pos
		keyEnd, err := scanString(masked, pos)
		if err != nil {
			return Object{}, err
		}
		var key string
		if err := json.Unmarshal(masked[keyStart:keyEnd], &key); err != nil {
			return Object{}, fmt.Errorf("bad key at offset %d: %w", keyStart, err)
		}

		pos = skipSpace(masked, keyEnd)
		if pos >= len(masked) || masked[pos] != ':' {
			return Object{}, fmt.Errorf("missing ':' after key at offset %d", keyEnd)
		}
		valueStart := skipSpace(masked, pos+1)
		valueEnd, err := scanValue(masked, valueStart)
		if err != nil {
			return Object{}, err
		}
		members = append(members, Member{Key: key, KeyStart: keyStart, ValueStart: valueStart, ValueEnd: valueEnd})
		pos = skipSpace(masked, valueEnd)
	}
}

// Array 是数组结构：左右方括号位置与每个元素的区间。
type Array struct {
	Open  int
	Close int
	Elems []Span
}

// scanArrayAt 从 open（必须指向 '['）开始扫描一个数组。
func scanArrayAt(masked []byte, open int) (Array, error) {
	if open >= len(masked) || masked[open] != '[' {
		return Array{}, fmt.Errorf("expected array at offset %d", open)
	}
	pos := skipSpace(masked, open+1)
	var elems []Span

	for {
		if pos >= len(masked) {
			return Array{}, fmt.Errorf("unterminated array at offset %d", open)
		}
		switch masked[pos] {
		case ']':
			return Array{Open: open, Close: pos, Elems: elems}, nil
		case ',':
			pos = skipSpace(masked, pos+1)
			continue
		}
		end, err := scanValue(masked, pos)
		if err != nil {
			return Array{}, err
		}
		elems = append(elems, Span{Start: pos, End: end})
		pos = skipSpace(masked, end)
	}
}

// Indent 按第一个成员所在行的缩进推断文件的排版风格；
// 空对象时沿用右花括号所在行的缩进。
func (o Object) Indent(masked []byte) string {
	start := o.Open + 1
	if len(o.Members) > 0 {
		start = o.Members[0].KeyStart
	}
	lineStart := start
	for lineStart > 0 && masked[lineStart-1] != '\n' {
		lineStart--
	}
	end := lineStart
	for end < len(masked) && (masked[end] == ' ' || masked[end] == '\t') {
		end++
	}
	indent := string(masked[lineStart:end])
	if strings.TrimSpace(indent) != "" {
		return "  "
	}
	return indent
}

// PatchOptions 控制 PatchMember 的追加行为。
type PatchOptions struct {
	// DropGapContaining 非空时，若"最后一个成员到右花括号"之间的原文含该子串，
	// 追加成员时丢弃这段空隙。典型场景：示例配置里被整段注释掉的同名键，
	// 写入真键后留着那段注释会让人以为还要手工取消注释。
	DropGapContaining string
}

// PatchMember 在 JSONC 原文 raw 上替换或追加顶层成员 key：
// 已有该键只换值，其余字节（注释、缩进、字段顺序）原样保留。
//
// render 按文件自身的缩进风格生成新的值，只在需要时调用一次，
// 因此调用方不必先自己扫一遍文件。
func PatchMember(raw []byte, key string, render func(indent string) ([]byte, error), opt PatchOptions) ([]byte, error) {
	masked := StripComments(raw)
	obj, err := ScanObject(masked)
	if err != nil {
		return nil, err
	}

	indent := obj.Indent(masked)
	value, err := render(indent)
	if err != nil {
		return nil, err
	}

	for _, m := range obj.Members {
		if m.Key != key {
			continue
		}
		out := make([]byte, 0, len(raw))
		out = append(out, raw[:m.ValueStart]...)
		out = append(out, value...)
		out = append(out, raw[m.ValueEnd:]...)
		return out, nil
	}

	// 插到最后一个成员之后。原文件里最后一个成员后面可能已经有一个逗号
	// （含尾随逗号的写法），吃掉它再补自己的逗号，避免出现两个逗号。
	insertAt := obj.Open + 1
	hasComma := false
	if len(obj.Members) > 0 {
		insertAt = obj.Members[len(obj.Members)-1].ValueEnd
		if p := skipSpace(masked, insertAt); p < len(masked) && masked[p] == ',' {
			hasComma = true
			insertAt = p + 1
		}
	}

	block := "\n\n" + indent + quoteKey(key) + ": " + string(value) + "\n"
	if len(obj.Members) > 0 && !hasComma {
		block = "," + block
	}

	gap := raw[insertAt:obj.Close]
	out := make([]byte, 0, len(raw)+len(block))
	out = append(out, raw[:insertAt]...)
	out = append(out, block...)
	if opt.DropGapContaining == "" || !strings.Contains(string(gap), opt.DropGapContaining) {
		out = append(out, gap...)
	}
	out = append(out, raw[obj.Close:]...)
	return out, nil
}

func quoteKey(key string) string {
	data, err := json.Marshal(key)
	if err != nil {
		return `"` + key + `"`
	}
	return string(data)
}

// scanString 返回字符串结束引号之后的位置。
func scanString(src []byte, start int) (int, error) {
	if start >= len(src) || src[start] != '"' {
		return 0, fmt.Errorf("expected string at offset %d", start)
	}
	escaped := false
	for i := start + 1; i < len(src); i++ {
		switch {
		case escaped:
			escaped = false
		case src[i] == '\\':
			escaped = true
		case src[i] == '"':
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated string at offset %d", start)
}

// scanValue 返回一个值结束之后的位置（对象/数组按括号配对）。
func scanValue(src []byte, start int) (int, error) {
	if start >= len(src) {
		return 0, fmt.Errorf("missing value")
	}
	switch src[start] {
	case '"':
		return scanString(src, start)
	case '{', '[':
		depth := 0
		for i := start; i < len(src); i++ {
			switch src[i] {
			case '"':
				end, err := scanString(src, i)
				if err != nil {
					return 0, err
				}
				i = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
		}
		return 0, fmt.Errorf("unterminated container at offset %d", start)
	default:
		i := start
		for i < len(src) && src[i] != ',' && src[i] != '}' && src[i] != ']' {
			i++
		}
		for i > start && isSpace(src[i-1]) {
			i--
		}
		if i == start {
			return 0, fmt.Errorf("empty value at offset %d", start)
		}
		return i, nil
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func skipSpace(src []byte, pos int) int {
	for pos < len(src) && isSpace(src[pos]) {
		pos++
	}
	return pos
}
