package jsonc

import (
	"errors"
	"fmt"
	"strings"
)

// Step 是 JSON 路径上的一步：对象里的键，或数组里的下标。
// 站点文件里的 location 是数组，所以两种都要支持。
type Step struct {
	Key     string
	Index   int
	IsIndex bool
	Append  bool // 只对数组有效，且必须是最后一步
}

// Key 构造"对象键"步骤。
func Key(k string) Step { return Step{Key: k} }

// At 构造"数组下标"步骤。
func At(i int) Step { return Step{Index: i, IsIndex: true} }

// Append 构造"追加到数组末尾"步骤，必须是最后一步。
func Append() Step { return Step{Append: true, IsIndex: true} }

// Edit 是一处修改：把 Path 指向的值换成 Value。
// Value 为 nil 表示删除该键——删不存在的键是空操作，不算错误。
type Edit struct {
	Path  []Step
	Value []byte
}

// Span 是原文里的一段字节区间 [Start, End)。
type Span struct{ Start, End int }

// PathString 把路径渲染成 location[0].root 这样的形式，用于报错。
func (e Edit) PathString() string {
	var b strings.Builder
	for i, s := range e.Path {
		switch {
		case s.Append:
			b.WriteString("[+]")
		case s.IsIndex:
			fmt.Fprintf(&b, "[%d]", s.Index)
		default:
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s.Key)
		}
	}
	return b.String()
}

// PatchPath 依次应用 edits。每一步都在上一步的结果上重新扫描：
// 一次要改的键很少，重新扫描比维护偏移量更不容易错。
//
// 只改被点名的字节，其余内容——注释、缩进、字段顺序、以及调用方没有建模的
// 字段——原样保留。这正是站点文件不被 site 命令吃掉手写配置的原因。
func PatchPath(raw []byte, edits []Edit) ([]byte, error) {
	out := raw
	for _, e := range edits {
		next, err := applyEdit(out, e)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.PathString(), err)
		}
		out = next
	}
	return out, nil
}

func applyEdit(raw []byte, e Edit) ([]byte, error) {
	if len(e.Path) == 0 {
		return nil, errors.New("路径为空")
	}

	masked := StripComments(raw)
	pos := skipSpace(masked, 0)
	if pos >= len(masked) || masked[pos] != '{' {
		return nil, errors.New("根节点必须是对象")
	}

	for _, step := range e.Path[:len(e.Path)-1] {
		next, err := descend(masked, pos, step)
		if err != nil {
			return nil, err
		}
		pos = next
	}

	last := e.Path[len(e.Path)-1]
	switch {
	case last.Append:
		arr, err := scanArrayAt(masked, pos)
		if err != nil {
			return nil, err
		}
		if e.Value == nil {
			return nil, errors.New("不支持删除数组元素")
		}
		return insertElem(raw, masked, arr, e.Value), nil

	case last.IsIndex:
		arr, err := scanArrayAt(masked, pos)
		if err != nil {
			return nil, err
		}
		if last.Index < 0 || last.Index >= len(arr.Elems) {
			return nil, fmt.Errorf("下标越界：%d（共 %d 个元素）", last.Index, len(arr.Elems))
		}
		if e.Value == nil {
			return nil, errors.New("不支持删除数组元素")
		}
		return replaceSpan(raw, arr.Elems[last.Index], e.Value), nil

	default:
		obj, err := scanObjectAt(masked, pos)
		if err != nil {
			return nil, err
		}
		for _, m := range obj.Members {
			if m.Key != last.Key {
				continue
			}
			if e.Value == nil {
				return deleteMember(raw, masked, m), nil
			}
			return replaceSpan(raw, Span{m.ValueStart, m.ValueEnd}, e.Value), nil
		}
		if e.Value == nil {
			return raw, nil
		}
		return insertMember(raw, masked, obj, last.Key, e.Value), nil
	}
}

// descend 沿着一步走到子值的起始位置。
func descend(masked []byte, pos int, step Step) (int, error) {
	if step.Append {
		return 0, errors.New("Append 只能是最后一步")
	}
	if pos >= len(masked) {
		return 0, errors.New("路径超出文档末尾")
	}

	switch masked[pos] {
	case '{':
		obj, err := scanObjectAt(masked, pos)
		if err != nil {
			return 0, err
		}
		if step.IsIndex {
			return 0, errors.New("路径要求数组，但这里是对象")
		}
		for _, m := range obj.Members {
			if m.Key == step.Key {
				return m.ValueStart, nil
			}
		}
		return 0, fmt.Errorf("找不到键 %q", step.Key)

	case '[':
		arr, err := scanArrayAt(masked, pos)
		if err != nil {
			return 0, err
		}
		if !step.IsIndex {
			return 0, fmt.Errorf("这里是数组，不能按键 %q 下降", step.Key)
		}
		if step.Index < 0 || step.Index >= len(arr.Elems) {
			return 0, fmt.Errorf("下标越界：%d（共 %d 个元素）", step.Index, len(arr.Elems))
		}
		return arr.Elems[step.Index].Start, nil
	}
	return 0, errors.New("路径中途遇到标量，无法继续下降")
}

func replaceSpan(raw []byte, sp Span, value []byte) []byte {
	out := make([]byte, 0, len(raw)-(sp.End-sp.Start)+len(value))
	out = append(out, raw[:sp.Start]...)
	out = append(out, value...)
	out = append(out, raw[sp.End:]...)
	return out
}

// deleteMember 删掉一个成员，连同它一侧的逗号，保证剩下的仍是合法 JSONC。
func deleteMember(raw, masked []byte, m Member) []byte {
	// 后面还有成员：吃掉自己的逗号，前一个成员的逗号留着。
	// 起删点回到行首，免得在原位留下一行空白。
	if p := skipSpace(masked, m.ValueEnd); p < len(masked) && masked[p] == ',' {
		return cut(raw, lineStart(masked, m.KeyStart), p+1)
	}
	// 自己是最后一个：连同前面那个逗号一起删，否则会剩下悬空的逗号。
	p := m.KeyStart - 1
	for p >= 0 && isSpace(masked[p]) {
		p--
	}
	if p >= 0 && masked[p] == ',' {
		return cut(raw, p, m.ValueEnd)
	}
	return cut(raw, lineStart(masked, m.KeyStart), m.ValueEnd)
}

// cut 删掉 [from, to) 这一段。
func cut(raw []byte, from, to int) []byte {
	out := make([]byte, 0, len(raw)-(to-from))
	out = append(out, raw[:from]...)
	out = append(out, raw[to:]...)
	return out
}

// lineStart 返回 pos 所在行的行首偏移；连行首那个换行也吃掉，
// 这样整行删掉后不会留下空行。
func lineStart(src []byte, pos int) int {
	for pos > 0 && src[pos-1] != '\n' {
		pos--
	}
	if pos > 0 && src[pos-1] == '\n' {
		pos--
	}
	return pos
}

// insertMember 在对象末尾插入一个新成员。排版规则与 PatchMember 一致：
// 空行分隔、沿用文件自身的缩进。
func insertMember(raw, masked []byte, obj Object, key string, value []byte) []byte {
	indent := obj.Indent(masked)
	insertAt := obj.Open + 1
	hasComma := false
	if len(obj.Members) > 0 {
		insertAt = obj.Members[len(obj.Members)-1].ValueEnd
		if p := skipSpace(masked, insertAt); p < len(masked) && masked[p] == ',' {
			hasComma = true
			insertAt = p + 1
		}
	}

	block := "\n" + indent + quoteKey(key) + ": " + indentValue(value, indent) + "\n"
	if len(obj.Members) > 0 && !hasComma {
		block = "," + block
	}

	out := make([]byte, 0, len(raw)+len(block))
	out = append(out, raw[:insertAt]...)
	out = append(out, block...)
	out = append(out, raw[insertAt:]...)
	return out
}

// insertElem 在数组末尾追加一个元素。
func insertElem(raw, masked []byte, arr Array, value []byte) []byte {
	indent := elemIndent(masked, arr)

	if len(arr.Elems) == 0 {
		// 空数组：元素占一行，右方括号回到数组所在行的缩进。
		block := "\n" + indent + indentValue(value, indent) + "\n" + lineIndent(masked, arr.Open)
		out := make([]byte, 0, len(raw)+len(block))
		out = append(out, raw[:arr.Open+1]...)
		out = append(out, block...)
		out = append(out, raw[arr.Open+1:]...)
		return out
	}

	insertAt := arr.Elems[len(arr.Elems)-1].End
	if p := skipSpace(masked, insertAt); p < len(masked) && masked[p] == ',' {
		insertAt = p + 1
	}
	block := ",\n" + indent + indentValue(value, indent)

	out := make([]byte, 0, len(raw)+len(block))
	out = append(out, raw[:insertAt]...)
	out = append(out, block...)
	out = append(out, raw[insertAt:]...)
	return out
}

// elemIndent 推断数组元素的缩进：有元素就取第一个元素所在行的缩进，
// 空数组则取左方括号所在行的缩进再加两个空格。
func elemIndent(masked []byte, arr Array) string {
	if len(arr.Elems) > 0 {
		return lineIndent(masked, arr.Elems[0].Start)
	}
	return lineIndent(masked, arr.Open) + "  "
}

// lineIndent 返回 pos 所在行的前导空白（行首不是空白时返回空串）。
func lineIndent(src []byte, pos int) string {
	start := pos
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := start
	for end < len(src) && (src[end] == ' ' || src[end] == '\t') {
		end++
	}
	return string(src[start:end])
}

// indentValue 给多行值补上缩进，保证插入后每行都对齐。
func indentValue(value []byte, indent string) string {
	lines := strings.Split(string(value), "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
