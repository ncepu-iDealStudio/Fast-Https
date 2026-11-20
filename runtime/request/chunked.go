package request

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// ErrChunkedBody is returned when a chunked body is malformed.
var ErrChunkedBody = errors.New("invalid chunked body")

// ErrBodyTooLarge 表示解码后的正文超过 max_body_size；调用方应回 413，
// 与「分块格式错误」区分开。
var ErrBodyTooLarge = errors.New("chunked body too large")

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

// HasTransferEncodingToken 报告 Transfer-Encoding 等非权重列表头是否包含指定 token。
// 与 HasToken 不同，Transfer-Encoding 的分号参数是 transfer-parameter 而非 q 权重，
// "Transfer-Encoding: chunked;q=0" 在上游仍按 chunked 解析，不能因 q=0 跳过分块解码。
func HasTransferEncodingToken(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := part
		if i := strings.IndexByte(part, ';'); i >= 0 {
			name = strings.TrimSpace(part[:i])
		}
		if strings.EqualFold(name, token) {
			return true
		}
	}
	return false
}

const (
	maxChunkHeaderLine  = 4096
	maxChunkTrailerSize = 8192
)

// ReadChunked decodes a chunked body.
// initial is the bytes already received after the header block.
// readMore returns the next socket read. max is the decoded-body limit.
func ReadChunked(initial []byte, readMore func() ([]byte, error), max int) ([]byte, error) {
	buf := append([]byte(nil), initial...)
	var out bytes.Buffer
	for {
		for !bytes.Contains(buf, []byte("\r\n")) {
			if len(buf) > maxChunkHeaderLine {
				return nil, ErrChunkedBody
			}
			more, err := readMore()
			if err != nil || len(more) == 0 {
				return nil, ErrChunkedBody
			}
			buf = append(buf, more...)
		}
		line, rest, _ := bytes.Cut(buf, []byte("\r\n"))
		if len(line) > maxChunkHeaderLine || bytes.ContainsAny(line, "\r\n") {
			return nil, ErrChunkedBody
		}
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
			if err := consumeChunkTrailers(buf, readMore); err != nil {
				return nil, err
			}
			return out.Bytes(), nil
		}
		if max > 0 && (n > uint64(max) || out.Len()+int(n) > max) {
			return nil, ErrBodyTooLarge
		}
		if n > uint64(^uint(0)>>1)-2 {
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

// consumeChunkTrailers 消费终止块 "0\r\n" 之后的可选 trailer 头与结束空行 "\r\n"。
func consumeChunkTrailers(buf []byte, readMore func() ([]byte, error)) error {
	total := 0
	for {
		for !bytes.Contains(buf, []byte("\r\n")) {
			if len(buf) > maxChunkTrailerSize {
				return ErrChunkedBody
			}
			more, err := readMore()
			if err != nil || len(more) == 0 {
				return ErrChunkedBody
			}
			buf = append(buf, more...)
		}
		line, rest, _ := bytes.Cut(buf, []byte("\r\n"))
		buf = rest
		if len(line) == 0 {
			return nil
		}
		total += len(line) + 2
		if total > maxChunkTrailerSize || bytes.IndexByte(line, ':') <= 0 || bytes.ContainsAny(line, "\r\n") {
			return ErrChunkedBody
		}
	}
}
