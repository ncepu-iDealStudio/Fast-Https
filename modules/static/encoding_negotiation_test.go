package static

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/runtime/listener"
)

// encodingCase 是 testdata/encoding-negotiation.jsonl 里的一行。
//
// zip 取值对应配置里的压缩开关，want 是期望选中的编码，空串表示不压缩。
type encodingCase struct {
	Zip    string `json:"zip"`
	Accept string `json:"accept"`
	Want   string `json:"want"`
	Note   string `json:"note"`
}

func loadEncodingCases(t *testing.T) []encodingCase {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "encoding-negotiation.jsonl"))
	if err != nil {
		t.Fatalf("open cases: %v", err)
	}
	defer f.Close()

	var out []encodingCase
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		var c encodingCase
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("testdata/encoding-negotiation.jsonl:%d: %v", line, err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan cases: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("testdata/encoding-negotiation.jsonl 是空的")
	}
	return out
}

func zipOf(t *testing.T, name string) uint16 {
	t.Helper()
	switch name {
	case "none":
		return config.ZIP_NONE
	case "gzip":
		return config.ZIP_GZIP
	case "br":
		return config.ZIP_BR
	case "gzip+br":
		return config.ZIP_GZIP_BR
	default:
		t.Fatalf("用例里的 zip=%q 不认识", name)
		return 0
	}
}

// TestEncodingWantedVectors 逐条跑协商表。
//
// 这里只测协商函数本身：它的输入是「配置的压缩开关 + 客户端的
// Accept-Encoding」，输出是选中的编码。真正写响应头由 getResBytes 负责，
// 单独用 TestStaticQZeroIsNotCompressed 端到端验证一次。
func TestEncodingWantedVectors(t *testing.T) {
	for _, c := range loadEncodingCases(t) {
		name := c.Zip + "/" + c.Accept
		if name == c.Zip+"/" {
			name = c.Zip + "/<empty>"
		}
		t.Run(strings.NewReplacer("/", "_", ";", "_", "=", "_", ",", "_", " ", "").Replace(name), func(t *testing.T) {
			got := encodingWanted(zipOf(t, c.Zip), c.Accept)
			if got != c.Want {
				t.Fatalf("%s：zip=%s accept=%q → %q，期望 %q",
					c.Note, c.Zip, c.Accept, got, c.Want)
			}
		})
	}
}

// TestStaticQZeroIsNotCompressed 端到端确认 q=0 时响应不带 Content-Encoding。
//
// 协商函数正确不代表链路正确：写响应头那段是另一处代码。
func TestStaticQZeroIsNotCompressed(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("q-zero-body ", 50)
	writeStaticFile(t, dir, "a.txt", []byte(body))
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_GZIP}

	resp := serveStatic(t, cfg, "GET", "gzip;q=0")
	if strings.Contains(resp, "Content-Encoding") {
		t.Fatalf("gzip;q=0 不应压缩，响应头为 %q", firstLine(resp))
	}
	if !strings.Contains(resp, body) {
		t.Fatal("未压缩时应返回原始内容")
	}
}

// TestStaticQZeroFallsBackToOtherEncoding 记录 gzip;q=0 但接受 br 时的行为：
// 配置了两种编码，应当改用 br 而不是整体放弃压缩。
func TestStaticQZeroFallsBackToOtherEncoding(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(strings.Repeat("fallback-body ", 50))
	writeStaticFile(t, dir, "a.txt", raw)
	cfg := &listener.ListenCfg{Path: "/", StaticRoot: dir, Zip: config.ZIP_GZIP_BR}

	resp := serveStatic(t, cfg, "GET", "gzip;q=0, br")
	if !strings.Contains(resp, "Content-Encoding: br") {
		t.Fatalf("gzip 被拒后应改用 br，响应头为 %q", firstLine(resp))
	}
	decodeBody(t, resp, "br", raw)
}
