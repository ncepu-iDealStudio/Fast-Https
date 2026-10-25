// 响应侧语料，来源（2026-10 抓取）：
//   - seanmonstar/httparse src/lib.rs 测试模块的响应向量（继承自 picohttpparser）
//     镜像：https://fastly.jsdelivr.net/gh/seanmonstar/httparse@master/src/lib.rs
//   - payload-box/http-request-smuggling-payloads TE.TE 头部混淆形态
//
// 与 httparse 的刻意分歧：
//   - 裸 LF 分行的响应一律拒绝（httparse 接受）：上游不可信，含 LF 的
//     状态行原样回写客户端即是响应拆分通道。
//   - reason 含 NUL/8-bit 字节时原样保留（httparse 清空 reason / 拒绝 NUL）：
//     不含 CR/LF 就没有拆分通道，按不透明字节透传。
package response

import (
	"strings"
	"testing"

	"fast-https/runtime/response"
)

var corpusInvalidHeaders = &ResponError{6, "Invalid headers"}
var corpusFirstLineInvalid = &ResponError{3, "Invalid first line"}

var responseCorpus = []struct {
	name       string
	raw        string
	wantErr    *ResponError // nil = 解析成功
	wantFirst  string
	wantHeader map[string]string
}{
	{
		name: "test_response_simple", raw: "HTTP/1.1 200 OK\r\n\r\n",
		wantFirst: "HTTP/1.1 200 OK",
	},
	{
		// 分歧：httparse 接受裸 LF 分行；我们拒绝（响应拆分通道）
		name: "test_response_newlines_lf_only", raw: "HTTP/1.0 403 Forbidden\nServer: foo.bar\n\n",
		wantErr: corpusFirstLineInvalid,
	},
	{
		name: "test_response_reason_missing", raw: "HTTP/1.1 200 \r\n\r\n",
		wantFirst: "HTTP/1.1 200 ",
	},
	{
		// 无 reason 且无尾随空格：同样合法（空 reason）
		name: "test_response_reason_missing_no_space", raw: "HTTP/1.1 200\r\n\r\n",
		wantFirst: "HTTP/1.1 200 ",
	},
	{
		name: "test_response_reason_missing_no_space_with_headers", raw: "HTTP/1.1 200\r\nFoo: bar\r\n\r\n",
		wantFirst:  "HTTP/1.1 200 ",
		wantHeader: map[string]string{"Foo": "bar"},
	},
	{
		// reason 中的制表符与空格都是 reason 的一部分
		name: "test_response_reason_with_space_and_tab", raw: "HTTP/1.1 101 Switching Protocols\t\r\n\r\n",
		wantFirst: "HTTP/1.1 101 Switching Protocols\t",
	},
	{
		// 多词 reason 必须整体保留，不得截断到第一个词
		name: "test_response_multiline_reason", raw: "HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\n\r\n",
		wantFirst:  "HTTP/1.1 503 Service Unavailable",
		wantHeader: map[string]string{"Connection": "close"},
	},
	{
		// 分歧：obs-text 字节按不透明透传（httparse 清空 reason）
		name: "test_response_reason_with_obsolete_text_byte", raw: "HTTP/1.1 200 X\xffZ\r\n\r\n",
		wantFirst: "HTTP/1.1 200 X\xffZ",
	},
	{
		// 分歧：NUL 无 CR/LF 即无拆分通道，透传（httparse 拒绝）
		name: "test_response_reason_with_nul_byte", raw: "HTTP/1.1 200 \x00\r\n\r\n",
		wantFirst: "HTTP/1.1 200 \x00",
	},
	{
		// 分歧：httparse 对不完整状态行返回 Partial；HttpResParse 是
		// 整串解析器（调用方读尽上游后才喂进来），不完整即非法
		name: "test_response_version_missing_space", raw: "HTTP/1.1",
		wantErr: corpusFirstLineInvalid,
	},
	{
		name: "test_response_empty_lines_prefix_lf_only", raw: "\n\nHTTP/1.1 200 OK\n\n",
		wantErr: corpusFirstLineInvalid,
	},
	{
		name: "test_response_no_cr", raw: "HTTP/1.0 200\nContent-type: text/html\n\n",
		wantErr: corpusFirstLineInvalid,
	},
	{
		// httparse test_forbid_response_with_whitespace_between_header_name_and_colon
		// + RFC 7230 §3.2.4：冒号前空白必须拒绝（TE 混淆走私形态）
		name:    "forbid_whitespace_between_header_name_and_colon",
		raw:     "HTTP/1.1 200 OK\r\nAccess-Control-Allow-Credentials : true\r\nBread: baguette\r\n\r\n",
		wantErr: corpusInvalidHeaders,
	},
	{
		// obs-fold 折行残迹：行首空白拒绝
		name:    "forbid_leading_space_header_line",
		raw:     "HTTP/1.1 200 OK\r\n X-Folded: true\r\n\r\n",
		wantErr: corpusInvalidHeaders,
	},
	{
		// 头已到但终止空行未到：头照常解析，Body 保持为空
		name:       "test_response_partial_parses_headers_as_much_as_it_can",
		raw:        "HTTP/1.1 200 OK\r\nServer: yolo\r\n",
		wantFirst:  "HTTP/1.1 200 OK",
		wantHeader: map[string]string{"Server": "yolo"},
	},
}

func TestResponseCorpus(t *testing.T) {
	for _, tc := range responseCorpus {
		t.Run(tc.name, func(t *testing.T) {
			res := response.ResponseInit()
			err := res.HttpResParse(tc.raw)

			if tc.wantErr != nil {
				if err.Error() != tc.wantErr.Error() {
					t.Fatalf("HttpResParse() = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err.Error() != ResponseOk.Error() {
				t.Fatalf("HttpResParse() = %v, want ResponseOk", err)
			}
			if res.FirstLine != tc.wantFirst {
				t.Errorf("FirstLine = %q, want %q", res.FirstLine, tc.wantFirst)
			}
			for k, v := range tc.wantHeader {
				if res.Headers[k] != v {
					t.Errorf("Header[%s] = %q, want %q", k, res.Headers[k], v)
				}
			}
			// 多词 reason 的往返：解析结果必须能原样再序列化
			// （仅对状态行字节精确可还原的用例开；空 reason 无尾随
			// 空格的形态序列化时补空格，属有损但语义等价）
			if tc.name == "test_response_multiline_reason" {
				res.SetBody([]byte(""))
				if got := string(res.GenerateResponse()); got != tc.raw {
					t.Errorf("round-trip = %q, want %q", got, tc.raw)
				}
			}
		})
	}
}

// 默认响应构造器：状态行与基础头齐备
func TestDefaultResponses(t *testing.T) {
	test := response.DefaultTest()
	if test.FirstLine != "HTTP/1.1 200 OK" || len(test.GetBody()) == 0 {
		t.Fatalf("DefaultTest: %+v", test)
	}
	if got := test.GenerateResponse(); len(got) == 0 {
		t.Fatal("DefaultTest must serialize")
	}

	headerErr := response.DefaultServerHeaderError()
	if !strings.Contains(headerErr.FirstLine, "500") {
		t.Fatalf("DefaultServerHeaderError first line: %q", headerErr.FirstLine)
	}
}

// GetBody 返回响应体切片
func TestGetBody(t *testing.T) {
	res := response.ResponseInit()
	res.SetBody([]byte("payload"))
	if string(res.GetBody()) != "payload" {
		t.Fatalf("GetBody = %q", res.GetBody())
	}
}
