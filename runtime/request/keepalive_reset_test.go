package request

import (
	"os"
	"path/filepath"
	"testing"

	"fast-https/runtime/listener"
)

func readHTTP(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return raw
}

// TestKeepAliveRequestIsFullyReset 覆盖 keep-alive 复用同一个 Request 对象
// 时，上一个请求的状态不能串到下一个。
//
// events.parseRequest 的做法是每轮先 Req.Flush() 再 ParseHeader/ParseBody，
// 所以这里按同样的顺序驱动。只要 Flush 漏清某一项，第二个请求就会带着
// 第一个请求的 Cookie / Authorization / 查询串 / body —— 这类泄漏在
// keep-alive 下会把 A 用户的凭据带给 B 用户，属于安全问题。
func TestKeepAliveRequestIsFullyReset(t *testing.T) {
	first := readHTTP(t, "keepalive-first.http")
	second := readHTTP(t, "keepalive-second.http")

	req := RequestInit(false)
	if code := req.ParseHeader(first); code != RequestOk {
		t.Fatalf("parse first: %v", code)
	}
	// 查询串在 ParseHost 里解析；传零值 Listener 只为避开 nil 解引用，
	// 它的 Port 是空串，不会改写 Host。
	req.ParseHost(&listener.Listener{})
	if err := req.ParseBody(first); err != nil {
		t.Fatalf("parse first body: %v", err)
	}

	// 前提：第一个请求确实带来了这些状态，否则后面的断言没有意义
	if req.Headers["Cookie"] == "" || req.Headers["Authorization"] == "" {
		t.Fatalf("第一个请求的凭据没解析出来：%v", req.Headers)
	}
	if req.Query["x"] != "1" {
		t.Fatalf("第一个请求的查询串没解析出来：%v", req.Query)
	}
	if req.Body.Len() == 0 {
		t.Fatal("第一个请求的 body 没解析出来")
	}

	req.Flush()

	if code := req.ParseHeader(second); code != RequestOk {
		t.Fatalf("parse second: %v", code)
	}
	req.ParseHost(&listener.Listener{})
	if err := req.ParseBody(second); err != nil {
		t.Fatalf("parse second body: %v", err)
	}

	// 凭据与查询串不得残留
	for _, h := range []string{"Cookie", "Authorization", "Content-Type", "Content-Length"} {
		if v, ok := req.Headers[h]; ok {
			t.Fatalf("header %s 残留在第二个请求里：%q", h, v)
		}
	}
	if len(req.Query) != 0 {
		t.Fatalf("查询串残留：%v", req.Query)
	}
	if req.Body.Len() != 0 || req.BodyLen != 0 {
		t.Fatalf("body 残留：len=%d BodyLen=%d", req.Body.Len(), req.BodyLen)
	}

	// 标量字段应换成第二个请求的值
	if req.Method != "GET" || req.Path != "/b" || req.Protocol != "HTTP/1.1" {
		t.Fatalf("标量字段未更新：method=%q path=%q protocol=%q",
			req.Method, req.Path, req.Protocol)
	}
	if req.GetHost() != "second.example" {
		t.Fatalf("Host 未更新：%q", req.GetHost())
	}
}

// TestFlushClearsAuxiliaryFields 单独锁定 Flush 对辅助字段的处理。
//
// Encoding / HeaderLen / BodyLen 目前没有读取方，但它们是「随请求变化」的
// 状态，留在 Flush 之外等于给将来的调用方埋一个跨请求残留的坑。
func TestFlushClearsAuxiliaryFields(t *testing.T) {
	req := RequestInit(false)
	req.Encoding = []string{"gzip"}
	req.HeaderLen = 123
	req.BodyLen = 456

	req.Flush()

	if req.Encoding != nil {
		t.Fatalf("Encoding 未清：%v", req.Encoding)
	}
	if req.HeaderLen != 0 {
		t.Fatalf("HeaderLen 未清：%d", req.HeaderLen)
	}
	if req.BodyLen != 0 {
		t.Fatalf("BodyLen 未清：%d", req.BodyLen)
	}
}
