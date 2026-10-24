package core

import (
	"strings"
	"testing"

	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

func TestResponseBytesOmitsBodyForHead(t *testing.T) {
	ev := &Event{}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "HEAD"
	ev.RR.Res = response.ResponseInit()
	ev.RR.Res.SetFirstLine(200, "OK")
	ev.RR.Res.SetHeader("Content-Length", "5")
	ev.RR.Res.SetHeader("Content-Type", "text/plain")
	ev.RR.Res.SetBody([]byte("hello"))

	got := string(ev.ResponseBytes())
	if strings.Contains(got, "hello") {
		t.Fatalf("HEAD response included body: %q", got)
	}
	if !strings.Contains(got, "Content-Length: 5") {
		t.Fatalf("HEAD response lost length: %q", got)
	}
}

func TestResponseBytesKeepsBodyForGet(t *testing.T) {
	ev := &Event{}
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Res = response.ResponseInit()
	ev.RR.Res.SetFirstLine(200, "OK")
	ev.RR.Res.SetBody([]byte("hello"))

	if !strings.Contains(string(ev.ResponseBytes()), "hello") {
		t.Fatal("GET response dropped the body")
	}
}
