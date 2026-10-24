package listener

import (
	"crypto/tls"
	"testing"
)

// TestALPNOnlyAdvertisedForH2Listener 确认只有 listen 含 h2 的端口才通告 ALPN "h2"。
// 否则纯 ssl 端口会被浏览器协商成 h2，却由 HTTP/1.1 解析器处理。
func TestALPNOnlyAdvertisedForH2Listener(t *testing.T) {
	dir := t.TempDir()
	crt, key := writeSelfSigned(t, dir, "a.example")
	cfg := []ListenCfg{{
		ServerName: "a.example",
		SSL:        SSLkv{SslKey: crt, SslValue: key},
	}}

	cases := []struct {
		name string
		h2   bool
		want string
	}{
		{"listen 443 ssl", false, ""},
		{"listen 443 ssl h2", true, "h2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ln := listenSsl("127.0.0.1:0", newCertSet(), cfg, tc.h2, false)
			t.Cleanup(func() { _ = ln.Close() })
			go acceptUntilClose(ln)

			conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
				InsecureSkipVerify: true, // 自签名测试证书
				ServerName:         "a.example",
				NextProtos:         []string{"h2", "http/1.1"},
			})
			if err != nil {
				t.Fatalf("tls dial: %v", err)
			}
			defer conn.Close()

			if got := conn.ConnectionState().NegotiatedProtocol; got != tc.want {
				t.Fatalf("negotiated protocol = %q, want %q", got, tc.want)
			}
		})
	}
}
