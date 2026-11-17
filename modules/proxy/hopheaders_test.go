package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

// hopByHopCase 是 testdata/hop-by-hop-headers.json 里的一条输入。
// headers 是客户端发来的头，absent / present 是上游应当看不到 / 看得到的头。
type hopByHopCase struct {
	Name     string            `json:"name"`
	Upgrade  string            `json:"upgrade"`
	Protocol string            `json:"protocol"`
	Headers  map[string]string `json:"headers"`
	Absent   []string          `json:"absent"`
	Present  map[string]string `json:"present"`
}

// TestHopByHopHeaderCases 逐条核对上游实际收到的头：单跳头与 Connection
// 点名的扩展头必须消失，端到端头必须保留，websocket 的升级对必须重建，
// 客户端要求关闭时要给上游补一条 Connection: close。
func TestHopByHopHeaderCases(t *testing.T) {
	cases := readHopByHopCases(t)
	if len(cases) == 0 {
		t.Fatal("用例为空")
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			addr, got := echoUpstream(t)
			cfg := &listener.ListenCfg{ProxyAddr: addr, PreserveHost: true}

			ev := core.NewEvent(&listener.Listener{}, &stubConn{remote: "203.0.113.9"})
			ev.RR.Req = request.RequestInit(false)
			ev.RR.Req.Protocol = "HTTP/1.1"
			if c.Protocol != "" {
				ev.RR.Req.Protocol = c.Protocol
			}
			for k, v := range c.Headers {
				ev.RR.Req.SetHeader(k, v, cfg)
			}
			ev.Upgrade = c.Upgrade

			ChangeHead(cfg, ev)

			p, err := getProxy(ev, &ev.RR, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { releaseProxy(p) })
			if _, err := p.Write(ev.RR.Req.ByteRow()); err != nil {
				t.Fatal(err)
			}

			select {
			case raw := <-got:
				headers := rawHeaderMap(raw)
				for _, name := range c.Absent {
					if v, ok := headers[strings.ToLower(name)]; ok {
						t.Fatalf("%s 不应到达上游，实到 %q", name, v)
					}
				}
				for name, want := range c.Present {
					if got := headers[strings.ToLower(name)]; got != want {
						t.Fatalf("%s = %q, want %q", name, got, want)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("上游没有收到请求")
			}
		})
	}
}

func readHopByHopCases(t *testing.T) []hopByHopCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "hop-by-hop-headers.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []hopByHopCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("解析用例失败：%v", err)
	}
	return cases
}

// rawHeaderMap 把回显的原始 HTTP 头解析成「小写头名 → 值」。
func rawHeaderMap(raw string) map[string]string {
	headers := make(map[string]string)
	for i, line := range strings.Split(raw, "\r\n") {
		if i == 0 || line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return headers
}
