package static

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// acceptEncodingCase 是 testdata/accept-encoding.jsonl 里的一行。
type acceptEncodingCase struct {
	Name            string   `json:"name"`
	File            string   `json:"file"`
	Body            string   `json:"body"`
	Zip             string   `json:"zip"`
	Accept          string   `json:"accept"`
	Method          string   `json:"method"`
	Range           string   `json:"range"`
	MinLength       int      `json:"min_length"`
	SkipTypes       []string `json:"skip_types"`
	WantEncoding    string   `json:"want_encoding"`
	WantStatus      string   `json:"want_status"`
	WantContentType string   `json:"want_content_type"`
	CompareGet      bool     `json:"compare_get"`
	Note            string   `json:"note"`
}

func loadAcceptEncodingCases(t *testing.T) []acceptEncodingCase {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "accept-encoding.jsonl"))
	if err != nil {
		t.Fatalf("open cases: %v", err)
	}
	defer f.Close()

	var out []acceptEncodingCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		var c acceptEncodingCase
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("testdata/accept-encoding.jsonl:%d: %v", line, err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan cases: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("testdata/accept-encoding.jsonl 是空的")
	}
	return out
}

// expandBody 展开 repeat:<次数>:<片段> 写法，其他情况按字面内容使用。
func expandBody(spec string) []byte {
	rest, ok := strings.CutPrefix(spec, "repeat:")
	if !ok {
		return []byte(spec)
	}
	countStr, fragment, ok := strings.Cut(rest, ":")
	if !ok {
		return []byte(spec)
	}
	n, err := strconv.Atoi(countStr)
	if err != nil || n < 0 {
		return []byte(spec)
	}
	return []byte(strings.Repeat(fragment, n))
}

// serveStaticFile 用指定文件名与额外请求头跑一次 getResBytes，返回原始响应字节。
func serveStaticFile(t *testing.T, cfg *listener.ListenCfg, file, method, accept, rangeSpec string) string {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })
	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = method
	ev.RR.Req.Path = "/" + file
	if accept != "" {
		ev.RR.Req.Headers["Accept-Encoding"] = accept
	}
	if rangeSpec != "" {
		ev.RR.Req.Headers["Range"] = rangeSpec
	}
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	return readPipe(t, client, server, func() {
		if getResBytes(&dynlog.DynLogger{}, cfg, cfg.StaticRoot+"/"+file, "close", ev) != 1 {
			t.Error("expected file")
		}
	})
}

// headerValue 取响应头里某个字段的值（大小写不敏感），不存在返回空串。
func caseHeader(resp, name string) string {
	for _, line := range strings.Split(firstLine(resp), "\r\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func responseBody(resp string) []byte {
	if i := strings.Index(resp, "\r\n\r\n"); i >= 0 {
		return []byte(resp[i+4:])
	}
	return nil
}

// rangeBody 按用例里的单段 "bytes=start-end" 截出期望正文。
func rangeBody(t *testing.T, spec string, raw []byte) []byte {
	t.Helper()
	spec = strings.TrimPrefix(spec, "bytes=")
	startStr, endStr, ok := strings.Cut(spec, "-")
	if !ok {
		t.Fatalf("range %q 不是单段范围", spec)
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start < 0 || end < start || end >= len(raw) {
		t.Fatalf("range %q 超出用例正文", spec)
	}
	return raw[start : end+1]
}

// TestAcceptEncodingCases 端到端跑 R-07 的七条语义。
func TestAcceptEncodingCases(t *testing.T) {
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })
	config.Store(&config.Fast_Https{}, map[string]string{"txt": "text/plain", "png": "image/png"})

	for _, c := range loadAcceptEncodingCases(t) {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			dir := t.TempDir()
			raw := expandBody(c.Body)
			writeStaticFile(t, dir, c.File, raw)
			cfg := &listener.ListenCfg{
				Path:         "/",
				StaticRoot:   dir,
				Zip:          zipOf(t, c.Zip),
				ZipMinLength: c.MinLength,
				ZipSkipTypes: c.SkipTypes,
			}
			method := c.Method
			if method == "" {
				method = "GET"
			}
			resp := serveStaticFile(t, cfg, c.File, method, c.Accept, c.Range)

			wantStatus := c.WantStatus
			if wantStatus == "" {
				wantStatus = "200 OK"
			}
			if !strings.HasPrefix(resp, "HTTP/1.1 "+wantStatus) {
				t.Fatalf("%s：状态行 = %q，期望 %q", c.Note, firstLine(resp), wantStatus)
			}

			if got := caseHeader(resp, "Content-Encoding"); got != c.WantEncoding {
				t.Fatalf("%s：Content-Encoding = %q，期望 %q", c.Note, got, c.WantEncoding)
			}
			if c.WantContentType != "" {
				if got := caseHeader(resp, "Content-Type"); got != c.WantContentType {
					t.Fatalf("%s：Content-Type = %q，期望 %q", c.Note, got, c.WantContentType)
				}
			}
			// 只要配置了压缩，响应就应带 Vary，压缩与否都一样。
			if cfg.Zip != config.ZIP_NONE {
				if got := caseHeader(resp, "Vary"); !strings.Contains(got, "Accept-Encoding") {
					t.Fatalf("%s：Vary = %q，缺少 Accept-Encoding", c.Note, got)
				}
			}

			if method == "HEAD" {
				if len(responseBody(resp)) != 0 {
					t.Fatalf("%s：HEAD 不应写正文，实际 %d 字节", c.Note, len(responseBody(resp)))
				}
			} else {
				// 声明的 Content-Length 必须与实际正文一致（压缩后要改写）。
				body := responseBody(resp)
				if cl := caseHeader(resp, "Content-Length"); cl != "" {
					n, err := strconv.Atoi(cl)
					if err != nil {
						t.Fatalf("%s：Content-Length = %q: %v", c.Note, cl, err)
					}
					if n != len(body) {
						t.Fatalf("%s：Content-Length = %d，实际正文 %d 字节", c.Note, n, len(body))
					}
				}
				switch c.WantEncoding {
				case "gzip":
					zr, err := gzip.NewReader(bytes.NewReader(body))
					if err != nil {
						t.Fatalf("%s：gzip reader: %v", c.Note, err)
					}
					defer zr.Close()
					got, err := io.ReadAll(zr)
					if err != nil {
						t.Fatalf("%s：decode gzip: %v", c.Note, err)
					}
					if !bytes.Equal(got, raw) {
						t.Fatalf("%s：解压后内容与文件不一致", c.Note)
					}
				case "br":
					got, err := io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
					if err != nil {
						t.Fatalf("%s：decode br: %v", c.Note, err)
					}
					if !bytes.Equal(got, raw) {
						t.Fatalf("%s：解压后内容与文件不一致", c.Note)
					}
				default:
					want := raw
					// Range 用例只回请求的那一段，不与应用原文件比较。
					if c.Range != "" {
						want = rangeBody(t, c.Range, raw)
					}
					if !bytes.Equal(body, want) {
						t.Fatalf("%s：未压缩正文应原样返回", c.Note)
					}
				}
			}

			if c.CompareGet {
				get := serveStaticFile(t, cfg, c.File, "GET", c.Accept, "")
				if h, g := caseHeader(resp, "Content-Length"), caseHeader(get, "Content-Length"); h != g {
					t.Fatalf("%s：HEAD Content-Length = %q，GET = %q", c.Note, h, g)
				}
				if h, g := caseHeader(resp, "Content-Encoding"), caseHeader(get, "Content-Encoding"); h != g {
					t.Fatalf("%s：HEAD Content-Encoding = %q，GET = %q", c.Note, h, g)
				}
			}
		})
	}
}
