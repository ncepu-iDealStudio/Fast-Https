package request

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// ErrChunkedBody is returned when a chunked body is malformed or too large.
var ErrChunkedBody = errors.New("invalid chunked body")

// HeaderFold returns the first header whose name matches, ignoring case.
func (r *Request) HeaderFold(name string) string {
	for key, value := range r.Headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// QValue 返回列表头里 token 的 q 值与是否出现过。
//
// 未出现时返回 (0, false)，调用方据此区分「显式拒绝（q=0）」和「没提到」——
// 这个区别在协商 Accept-Encoding 的 `*` 通配符时必须知道（RFC 7231 §5.3.4：
// 显式写 q=0 的效果优先于 `*`）。同一 token 重复出现时取第一个。
func QValue(header, token string) (float64, bool) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		val := 1.0
		if i := strings.Index(part, ";"); i >= 0 {
			name = strings.TrimSpace(part[:i])
			for _, param := range strings.Split(part[i+1:], ";") {
				param = strings.TrimSpace(param)
				if strings.HasPrefix(param, "q=") {
					parsed, err := strconv.ParseFloat(param[2:], 64)
					if err == nil {
						val = parsed
					}
				}
			}
		}
		if strings.EqualFold(name, token) {
			return val, true
		}
	}
	return 0, false
}

// HasToken reports whether a list header contains token. q=0 does not count.
func HasToken(header, token string) bool {
	q, ok := QValue(header, token)
	return ok && q > 0
}

// ReadChunked decodes a chunked body.
// initial is the bytes already received after the header block.
// readMore returns the next socket read. max is the decoded-body limit.
func ReadChunked(initial []byte, readMore func() ([]byte, error), max int) ([]byte, error) {
	buf := append([]byte(nil), initial...)
	var out bytes.Buffer
	for {
		for !bytes.Contains(buf, []byte("\r\n")) {
			more, err := readMore()
			if err != nil || len(more) == 0 {
				return nil, ErrChunkedBody
			}
			buf = append(buf, more...)
		}
		line, rest, _ := bytes.Cut(buf, []byte("\r\n"))
		buf = rest
		sizeToken := line
		if i := bytes.IndexByte(line, ';'); i >= 0 {
			sizeToken = line[:i]
		}
		n, err := strconv.ParseUint(strings.TrimSpace(string(sizeToken)), 16, 64)
		if err != nil {
			return nil, ErrChunkedBody
		}
		if n == 0 {
			for len(buf) < 2 {
				more, err := readMore()
				if err != nil || len(more) == 0 {
					return nil, ErrChunkedBody
				}
				buf = append(buf, more...)
			}
			if !bytes.HasPrefix(buf, []byte("\r\n")) {
				return nil, ErrChunkedBody
			}
			return out.Bytes(), nil
		}
		if max > 0 && (n > uint64(max) || out.Len()+int(n) > max) {
			return nil, ErrChunkedBody
		}
		need := int(n) + 2
		for len(buf) < need {
			more, err := readMore()
			if err != nil || len(more) == 0 {
				return nil, ErrChunkedBody
			}
			buf = append(buf, more...)
		}
		if string(buf[n:n+2]) != "\r\n" {
			return nil, ErrChunkedBody
		}
		out.Write(buf[:n])
		buf = buf[need:]
	}
}
