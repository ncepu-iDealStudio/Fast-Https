// 语料不是手写的：全部取自公开权威语料库，覆盖性打到 ParseHeader /
// ReadChunked 上。来源（2026-10 抓取）：
//
//   - seanmonstar/httparse（Rust 生态事实标准的 HTTP 解析器，向量继承自
//     picohttpparser）内嵌于 src/lib.rs 测试模块的请求向量。
//     镜像：https://fastly.jsdelivr.net/gh/seanmonstar/httparse@master/src/lib.rs
//   - payload-box/http-request-smuggling-payloads 的
//     Intruder/Detection-Payloads.txt 与 Intruder/TE-TE-Payloads.txt
//     （CL.TE / TE.CL / TE.TE 走私 payload，含 TE 混淆变体族）。
//     https://github.com/payload-box/http-request-smuggling-payloads
//
// 与 httparse 的两处刻意分歧（防御性收紧，不是缺陷）：
//
//  1. 裸 LF 分行的请求一律拒绝（httparse 接受）：CRLF 是唯一合法行终止，
//     拒绝裸 LF 消灭整类请求走私向量。
//  2. 版本号等字段错误在"行尚未结束"时返回 NeedReadMore（与 httparse 的
//     Partial 一致），行结束（CRLF 到达）后才给出 ProtocolInvalid 等判定。
package request

import (
	"strings"
	"testing"

	"fast-https/runtime/request"
)

type verdict int

const (
	verdictOK verdict = iota // 解析成功
	verdictMore              // 头未读全，需要继续读
	verdictReject            // 拒绝
)

var httparseCases = []struct {
	name    string
	raw     string
	want    verdict
	wantErr *request.RequestError
	// verdictOK 时抽查解析结果
	wantMethod string
	wantPath   string
	wantHeader map[string]string
}{
	// ---- httparse: Ok(Status::Complete) 向量 ----
	{
		name: "test_request_simple", raw: "GET / HTTP/1.1\r\n\r\n",
		wantMethod: "GET", wantPath: "/",
	},
	{
		name: "test_request_simple_with_query_params", raw: "GET /thing?data=a HTTP/1.1\r\n\r\n",
		wantMethod: "GET", wantPath: "/thing?data=a",
	},
	{
		name: "test_request_simple_with_whatwg_query_params", raw: "GET /thing?data=a^ HTTP/1.1\r\n\r\n",
		wantMethod: "GET", wantPath: "/thing?data=a^",
	},
	{
		name:    "test_request_headers",
		raw:     "GET / HTTP/1.1\r\nHost: foo.com\r\nCookie: \r\n\r\n",
		wantMethod: "GET", wantPath: "/",
		wantHeader: map[string]string{"Host": "foo.com", "Cookie": ""},
	},
	{
		// 冒号后与值尾的 OWS（含 HTAB）必须剥掉
		name:    "test_request_headers_optional_whitespace",
		raw:     "GET / HTTP/1.1\r\nHost: \tfoo.com\t \r\nCookie: \t \r\n\r\n",
		wantHeader: map[string]string{"Host": "foo.com", "Cookie": ""},
	},
	{
		name:    "test_request_header_value_htab_short",
		raw:     "GET / HTTP/1.1\r\nUser-Agent: some\tagent\r\n\r\n",
		wantHeader: map[string]string{"User-Agent": "some\tagent"},
	},
	{
		name:    "test_request_header_value_htab_med",
		raw:     "GET / HTTP/1.1\r\nUser-Agent: 1234567890some\tagent\r\n\r\n",
		wantHeader: map[string]string{"User-Agent": "1234567890some\tagent"},
	},
	{
		name:    "test_request_header_value_htab_long",
		raw:     "GET / HTTP/1.1\r\nUser-Agent: 1234567890some\t1234567890agent1234567890\r\n\r\n",
		wantHeader: map[string]string{"User-Agent": "1234567890some\t1234567890agent1234567890"},
	},
	{
		// 冒号后紧跟值（无空格）合法
		name:    "test_request_header_no_space_after_colon",
		raw:     "GET / HTTP/1.1\r\nUser-Agent:omg-no-space1234567890some1234567890agent1234567890\r\n\r\n",
		wantHeader: map[string]string{"User-Agent": "omg-no-space1234567890some1234567890agent1234567890"},
	},
	{
		name:    "test_request_headers_max",
		raw:     "GET / HTTP/1.1\r\nA: A\r\nB: B\r\nC: C\r\nD: D\r\n\r\n",
		wantHeader: map[string]string{"A": "A", "B": "B", "C": "C", "D": "D"},
	},
	{
		name:    "test_request_multibyte",
		raw:     "GET / HTTP/1.1\r\nHost: foo.com\r\nUser-Agent: \xe3\x81\xb2\xe3/1.0\r\n\r\n",
		wantHeader: map[string]string{"User-Agent": "\xe3\x81\xb2\xe3/1.0"},
	},
	{
		// RFC 7230 §3.5：请求行前的空行应忽略
		name: "test_request_empty_lines_prefix", raw: "\r\n\r\nGET / HTTP/1.1\r\n\r\n",
		wantMethod: "GET", wantPath: "/",
	},
	{
		// 原向量为 LF 分行（我们按策略拒绝原样版，此处保留其意图：
		// 反斜杠在 path/query 中合法），转成 CRLF 分行验证
		name: "test_request_path_backslash_crlf_variant", raw: "GET /\\?wayne\\=5 HTTP/1.1\r\n\r\n",
		wantMethod: "GET", wantPath: "/\\?wayne\\=5",
	},

	// ---- httparse: Ok(Status::Partial) 向量：分片/未读全 ----
	{name: "test_request_one_byte_method", raw: "G", want: verdictMore},
	{name: "test_request_partial_method", raw: "GE", want: verdictMore},
	{name: "test_request_method_no_delimiter", raw: "GET", want: verdictMore},
	{name: "test_request_method_only", raw: "GET ", want: verdictMore},
	{name: "test_request_partial", raw: "GET / HTTP/1.1\r\n\r", want: verdictMore},
	{name: "test_request_partial_version", raw: "GET / HTTP/1.", want: verdictMore},
	{name: "test_request_method_path_no_delimiter", raw: "GET /", want: verdictMore},
	{name: "test_request_method_path_only", raw: "GET / ", want: verdictMore},
	// 非 httparse 向量：CRLF 被切在 TCP 段边界（前半 \r 已到，\n 未到）
	{name: "crlf_split_across_segments", raw: "GET / HTTP/1.1\r", want: verdictMore},
	{
		name: "test_request_partial_parses_headers_as_much_as_it_can",
		raw:  "GET / HTTP/1.1\r\nHost: yolo\r\n",
		want: verdictMore, wantMethod: "GET", wantPath: "/",
		wantHeader: map[string]string{"Host": "yolo"},
	},

	// ---- httparse: Err 向量 ----
	{
		name: "test_request_newlines_lf_only", raw: "GET / HTTP/1.1\nHost: foo.bar\n\n",
		want: verdictReject, wantErr: request.UnknowInvalid,
	},
	{
		name: "test_request_empty_lines_prefix_lf_only", raw: "\n\nGET / HTTP/1.1\n\n",
		want: verdictReject, wantErr: request.UnknowInvalid,
	},
	{
		name: "test_request_with_invalid_token_delimiter", raw: "GET\n/ HTTP/1.1\r\nHost: foo.bar\r\n\r\n",
		want: verdictReject, wantErr: request.FirstLineInvalid,
	},
	{
		// 原向量 "GET / HTTP/1!" 无行尾（httparse 立即报 Version 错）；
		// 我们在行未结束时等待更多数据（NeedReadMore，见上），行结束
		// 后立即判定。这里补齐 CRLF 验证终态判定：
		name: "test_request_with_invalid_but_short_version_completed", raw: "GET / HTTP/1!\r\n\r\n",
		want: verdictReject, wantErr: request.ProtocolInvalid,
	},
	{
		name: "test_request_with_empty_method", raw: " / HTTP/1.1\r\n\r\n",
		want: verdictReject, wantErr: request.MethodInvalid,
	},
	{
		name: "test_request_with_empty_path", raw: "GET  HTTP/1.1\r\n\r\n",
		want: verdictReject, wantErr: request.PathInvalid,
	},
	{
		name: "test_request_with_empty_method_and_path", raw: "  HTTP/1.1\r\n\r\n",
		want: verdictReject, wantErr: request.MethodInvalid,
	},
	// 孤立 CR 分行（原项目回归向量，走私变体）
	{
		name: "bare_cr_line_endings", raw: "GET / HTTP/1.1\r\rHost: example.com\r\rConnection: close\r\r",
		want: verdictReject, wantErr: request.UnknowInvalid,
	},
}

func TestHttparseRequestCorpus(t *testing.T) {
	for _, tc := range httparseCases {
		t.Run(tc.name, func(t *testing.T) {
			req := request.RequestInit(false)
			err := req.ParseHeader([]byte(tc.raw))

			switch tc.want {
			case verdictOK:
				if err != request.RequestOk {
					t.Fatalf("ParseHeader() = %v, want RequestOk", err)
				}
			case verdictMore:
				if err != request.RequestNeedReadMore {
					t.Fatalf("ParseHeader() = %v, want RequestNeedReadMore", err)
				}
			case verdictReject:
				if err != tc.wantErr {
					t.Fatalf("ParseHeader() = %v, want %v", err, tc.wantErr)
				}
				return
			}

			if tc.wantMethod != "" && req.Method != tc.wantMethod {
				t.Errorf("Method = %q, want %q", req.Method, tc.wantMethod)
			}
			if tc.wantPath != "" && req.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", req.Path, tc.wantPath)
			}
			for k, v := range tc.wantHeader {
				if req.Headers[k] != v {
					t.Errorf("Header[%s] = %q, want %q", k, req.Headers[k], v)
				}
			}
			// 反注入不变量：任何已解析字段都不含裸 CR/LF
			if strings.ContainsAny(req.Method+req.Path+req.Protocol, "\r\n") {
				t.Errorf("CR/LF leaked into request line fields: %+v", req)
			}
			for k, v := range req.Headers {
				if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
					t.Errorf("CR/LF leaked into header %q: %q", k, v)
				}
			}
		})
	}
}

// TestHttparseRequestPartialPermutations 对应 httparse 的 partial_permutations：
// 合法请求的任意前缀都必须判 NeedReadMore（分片到达不得误拒）。
func TestHttparseRequestPartialPermutations(t *testing.T) {
	const reqStr = "GET / HTTP/1.1\r\nHost: a\r\n\r\n"
	for i := 1; i < len(reqStr); i++ {
		req := request.RequestInit(false)
		err := req.ParseHeader([]byte(reqStr[:i]))
		if err != request.RequestNeedReadMore {
			t.Fatalf("prefix %q: ParseHeader() = %v, want RequestNeedReadMore", reqStr[:i], err)
		}
	}
}

// ---- payload-box 走私语料 ----
//
// 我们的反走私立场：语法合法的 CL+TE / TE+TE 混淆头不靠"识别恶意"来防，
// 而是（a）解析层拒绝 RFC 明确要求拒绝的形式（冒号前空白、obs-fold 折行）；
// （b）重复头折叠为单值（后值胜），报文经 ByteRow 从折叠结果重建，
// 上游看到的就是我们解析到的——前后端不存在第二种解读，走私无从发生。
var smugglingCases = []struct {
	name     string
	raw      string // 完整请求，CRLF 分行，含 body
	rejected bool   // true = ParseHeader 应拒绝
	wantTE   string // 折叠后的 Transfer-Encoding（不存在头时为 ""，不校验）
	framing  string // 我们实际采用的消息框架："chunked" 或 "cl"
}{
	{
		// Detection-Payloads #1（CL.TE 检测型）
		name: "cl_te_basic",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Type: application/x-www-form-urlencoded\r\n" +
			"Content-Length: 6\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"\r\n" +
			"0\r\n\r\nX",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// Detection-Payloads #4（TE.CL 检测型）
		name: "te_cl_basic",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Type: application/x-www-form-urlencoded\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Content-Length: 4\r\n" +
			"\r\n" +
			"1\r\nA\r\n0\r\n\r\n",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// TE-TE #1：重复 TE，后者非 chunked → 折叠为 "x"，框架回落 CL
		name: "te_te_duplicate_falls_back_to_cl",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding: x\r\n" +
			"\r\n" +
			"44\r\nGET /404 HTTP/1.1\r\nX-Test-Header: smuggled\r\n\r\n0\r\n\r\n",
		wantTE: "x", framing: "cl",
	},
	{
		// TE-TE #16：两个完全相同的 TE → 折叠后仍是 chunked
		name: "te_te_identical_duplicates",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// TE-TE 混淆变体：冒号前空格 → RFC 7230 §3.2.4 MUST 拒绝
		name: "te_te_space_before_colon",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding : chunked\r\n" +
			"\r\n0\r\n\r\n",
		rejected: true,
	},
	{
		// Detection #17：行首空格的 TE（obs-fold 折行残迹）→ 拒绝
		name: "te_leading_space_line",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			" Transfer-Encoding: chunked\r\n" +
			"\r\n2e\r\nGET /404 HTTP/1.1\r\nHost: v\r\n\r\n0\r\n\r\n",
		rejected: true,
	},
	{
		// TE-TE obs-fold：头名与冒号被折到两行 → 第一行无冒号，拒绝
		name: "te_folded_across_lines",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding\r\n" +
			": chunked\r\n" +
			"\r\n0\r\n\r\n",
		rejected: true,
	},
	{
		// TE-TE #9：冒号后紧跟值（无空格）合法
		name: "te_no_space_after_colon",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding:chunked\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// TE-TE #11：冒号后是 TAB → OWS，合法，值剥空白后仍是 chunked
		name: "te_tab_after_colon",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding:\tchunked\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// TE-TE #2：大小写变体 → canonical 折叠到同名头，后值胜
		name: "te_case_variant",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-encoding: chunked\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "chunked", framing: "chunked",
	},
	{
		// TE-TE #33：带引号的 chunked 不是 chunked token → 回落 CL
		name: "te_quoted_not_token",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding: \"chunked\"\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "\"chunked\"", framing: "cl",
	},
	{
		// TE-TE #41：中间空格切开 token → 非 chunked → 回落 CL
		name: "te_broken_token",
		raw: "POST / HTTP/1.1\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 4\r\n" +
			"Transfer-Encoding: chunked\r\n" +
			"Transfer-Encoding: chu nked\r\n" +
			"\r\n0\r\n\r\n",
		wantTE: "chu nked", framing: "cl",
	},
	{
		// Detection（HTTP/2 降级文本形态）：HTTP/2 是合法协议串，
		// 夹带的第二个请求行只会成为 body 字节
		name: "h2_downgrade_text_smuggle",
		raw: "POST / HTTP/2\r\n" +
			"Host: vulnerable-website.com\r\n" +
			"Content-Length: 0\r\n" +
			"\r\n" +
			"GET /404 HTTP/1.1\r\nHost: vulnerable-website.com\r\n\r\n",
		framing: "cl",
	},
	{
		// Detection（HTTP/2 伪头文本形态）：":method" 冒号开头 = 空头名，拒绝
		name: "h2_pseudo_header_in_text",
		raw: "POST / HTTP/2\r\n" +
			"Host: vulnerable-website.com\r\n" +
			":method: POST\r\n" +
			":path: /\r\n" +
			"content-length: 0\r\n" +
			"\r\n",
		rejected: true,
	},
}

func TestSmugglingCorpus(t *testing.T) {
	for _, tc := range smugglingCases {
		t.Run(tc.name, func(t *testing.T) {
			req := request.RequestInit(false)
			err := req.ParseHeader([]byte(tc.raw))

			if tc.rejected {
				if err == request.RequestOk || err == request.RequestNeedReadMore {
					t.Fatalf("smuggling payload was accepted: %v", err)
				}
				return
			}
			if err != request.RequestOk {
				t.Fatalf("ParseHeader() = %v, want RequestOk", err)
			}

			// 走私核心不变量：外层请求字段不被 body 里的第二个请求行污染
			if req.Method != "POST" || req.Path != "/" {
				t.Fatalf("outer request mutated: %s %s", req.Method, req.Path)
			}
			for k, v := range req.Headers {
				if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
					t.Errorf("CR/LF leaked into header %q", k)
				}
			}

			te := req.HeaderFold("Transfer-Encoding")
			if tc.wantTE != "" && te != tc.wantTE {
				t.Errorf("folded Transfer-Encoding = %q, want %q", te, tc.wantTE)
			}
			chunked := request.HasToken(te, "chunked")
			if tc.framing == "chunked" && !chunked {
				t.Errorf("framing = cl (TE=%q), want chunked", te)
			}
			if tc.framing == "cl" && chunked {
				t.Errorf("framing = chunked (TE=%q), want cl", te)
			}
		})
	}
}

// TestSmuggledPrefixNeverParsed：chunked 框架下，终止 chunk 之后的字节
// 属于"下一个请求"的走私载荷——ReadChunked 必须在终止 chunk 处停下，
// 绝不能把它解码进 body 或继续当请求消费。
func TestSmuggledPrefixNeverParsed(t *testing.T) {
	const raw = "POST / HTTP/1.1\r\n" +
		"Host: vulnerable-website.com\r\n" +
		"Content-Length: 6\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"0\r\n\r\n" +
		"GET /404 HTTP/1.1\r\nX-Ignore: X\r\n\r\n"

	req := request.RequestInit(false)
	if err := req.ParseHeader([]byte(raw)); err != request.RequestOk {
		t.Fatalf("ParseHeader() = %v", err)
	}
	_ = req.ParseBody([]byte(raw)) // body = 终止 chunk 之后的一切

	// 按 TE=chunked 解码：在 "0\r\n\r\n" 处终止，走私前缀留在未消费区
	body, err := request.ReadChunked(req.Body.Bytes(), func() ([]byte, error) {
		return nil, nil // 全部数据已在 initial 里
	}, 1<<20)
	if err != nil {
		t.Fatalf("ReadChunked() = %v", err)
	}
	if len(body) != 0 {
		t.Errorf("decoded body = %q, want empty", body)
	}
	if !strings.HasPrefix(string(req.Body.Bytes()), "0\r\n\r\n") {
		t.Errorf("body region = %q, want it to start with terminal chunk", req.Body.Bytes())
	}
}

// TestChunkSizeCorpus：httparse parse_chunk_size 向量族对应的
// ReadChunked 行为（chunk 扩展、超限、垃圾行）。
func TestChunkSizeCorpus(t *testing.T) {
	t.Run("chunk extension is not part of size", func(t *testing.T) {
		// 对应向量 "3735AB1;foo bar*\r\n"：分号后是扩展，不参与长度
		body, err := request.ReadChunked([]byte("5;x=y\r\nhello\r\n0\r\n\r\n"), func() ([]byte, error) {
			return nil, nil
		}, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "hello" {
			t.Fatalf("body = %q", body)
		}
	})
	t.Run("huge size line parses then hits cap", func(t *testing.T) {
		// 对应向量 "3086d\r\n"（198765 字节）：尺寸行本身合法，
		// 但超过 max 上限时必须拒绝
		_, err := request.ReadChunked([]byte("3086d\r\n"), func() ([]byte, error) {
			return nil, nil
		}, 1024)
		if err != request.ErrChunkedBody {
			t.Fatalf("err = %v, want ErrChunkedBody", err)
		}
	})
	t.Run("garbage without crlf is rejected", func(t *testing.T) {
		// 对应向量 "567f8a\rfoo"（CR 后不是 LF）
		_, err := request.ReadChunked([]byte("567f8a\rfoo"), func() ([]byte, error) {
			return nil, nil // eof
		}, 1024)
		if err != request.ErrChunkedBody {
			t.Fatalf("err = %v, want ErrChunkedBody", err)
		}
	})
	t.Run("bare hex size with body", func(t *testing.T) {
		// 对应向量 "12\r\nchunk"（0x12=18 字节）
		payload := "12\r\n" + strings.Repeat("a", 18) + "\r\n0\r\n\r\n"
		body, err := request.ReadChunked([]byte(payload), func() ([]byte, error) {
			return nil, nil
		}, 1024)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != 18 {
			t.Fatalf("len(body) = %d, want 18", len(body))
		}
	})
}
