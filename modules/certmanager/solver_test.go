package certmanager

import (
	"context"
	"crypto/tls"
	"fast-https/core"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
	"testing"

	"github.com/mholt/acmez/v3/acme"
)

func TestHTTP01SolverPresentLookupCleanUp(t *testing.T) {
	solver := newHTTP01Solver()
	chal := acme.Challenge{Token: "token-1", KeyAuthorization: "token-1.thumbprint"}

	if _, ok := solver.lookup("token-1"); ok {
		t.Fatal("token table should start empty")
	}
	if err := solver.Present(context.Background(), chal); err != nil {
		t.Fatalf("present: %v", err)
	}
	got, ok := solver.lookup("token-1")
	if !ok || got != "token-1.thumbprint" {
		t.Fatalf("lookup = %q, %v", got, ok)
	}
	if err := solver.CleanUp(context.Background(), chal); err != nil {
		t.Fatalf("clean up: %v", err)
	}
	if _, ok := solver.lookup("token-1"); ok {
		t.Fatal("clean up should drop the token")
	}
}

func TestHTTP01SolverAnswersChallengePath(t *testing.T) {
	solver := newHTTP01Solver()
	if err := solver.Present(context.Background(), acme.Challenge{
		Token:            "tok-abc",
		KeyAuthorization: "tok-abc.keyauth",
	}); err != nil {
		t.Fatalf("present: %v", err)
	}

	ev := challengeEvent("/.well-known/acme-challenge/tok-abc")
	if !solver.handle(ev) {
		t.Fatal("challenge request should be answered")
	}
	if got := ev.RR.Res.FirstLine; got != "HTTP/1.1 200 OK" {
		t.Fatalf("first line = %q", got)
	}
	if got := string(ev.RR.Res.GetBody()); got != "tok-abc.keyauth" {
		t.Fatalf("body = %q", got)
	}
	if got := ev.RR.Res.Headers["Content-Type"]; got != "text/plain" {
		t.Fatalf("content type = %q", got)
	}

	// 未知 token 不能劫持站点自己的 challenge 目录。
	unknown := challengeEvent("/.well-known/acme-challenge/other-token")
	if solver.handle(unknown) {
		t.Fatal("unknown token must fall through to normal routing")
	}
	// 其它路径不受影响。
	if solver.handle(challengeEvent("/index.html")) {
		t.Fatal("unrelated path must not be handled")
	}
	// 多段路径不是 ACME 发出的请求。
	if solver.handle(challengeEvent("/.well-known/acme-challenge/a/b")) {
		t.Fatal("nested path must fall through")
	}
}

func TestTLSALPN01SolverCertificate(t *testing.T) {
	solver := newTLSALPN01Solver()
	chal := acme.Challenge{
		Identifier:       acme.Identifier{Type: "dns", Value: "alpn.example"},
		Token:            "token",
		KeyAuthorization: "token.thumbprint",
	}
	if err := solver.Present(context.Background(), chal); err != nil {
		t.Fatalf("present: %v", err)
	}

	hello := &tls.ClientHelloInfo{ServerName: "alpn.example", SupportedProtos: []string{"acme-tls/1"}}
	cert, ok := solver.certificate(hello)
	if !ok || cert == nil {
		t.Fatal("challenge certificate should be served for acme-tls/1")
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("challenge certificate has no DER bytes")
	}

	// 客户端没带 acme-tls/1：普通握手，必须回落站点证书。
	if _, ok := solver.certificate(&tls.ClientHelloInfo{ServerName: "alpn.example"}); ok {
		t.Fatal("challenge certificate must not be served without acme-tls/1")
	}
	// SNI 未命中。
	if _, ok := solver.certificate(&tls.ClientHelloInfo{ServerName: "other.example", SupportedProtos: []string{"acme-tls/1"}}); ok {
		t.Fatal("unknown SNI must not be served")
	}

	if err := solver.CleanUp(context.Background(), chal); err != nil {
		t.Fatalf("clean up: %v", err)
	}
	if _, ok := solver.certificate(hello); ok {
		t.Fatal("clean up should drop the challenge certificate")
	}
}

// challengeEvent 构造一个只用于挑战应答的最小事件：写响应走桩函数，
// 不碰真实连接。
func challengeEvent(path string) *core.Event {
	ev := core.NewEvent(nil, nil)
	ev.RR.Req = &request.Request{Method: "GET", Path: path}
	ev.RR.Res = response.ResponseInit()
	ev.EventWrite = func(*core.Event, []byte) error { return nil }
	return ev
}
