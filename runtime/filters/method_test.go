package filters

import (
	"fast-https/runtime/listener"
	"testing"
)

func TestMethodAllowed(t *testing.T) {
	unrestricted := &listener.ListenCfg{}
	for _, method := range []string{"GET", "POST", "DELETE"} {
		if !MethodAllowed(unrestricted, method) {
			t.Fatalf("%s should pass without allow_methods", method)
		}
	}

	readOnly := &listener.ListenCfg{AllowMethods: []string{"GET"}}
	if !MethodAllowed(readOnly, "GET") {
		t.Fatal("GET should be allowed")
	}
	// HEAD 随 GET 放行：RFC 9110 要求 GET 支持的资源也要支持 HEAD。
	if !MethodAllowed(readOnly, "HEAD") {
		t.Fatal("HEAD should follow GET")
	}
	if MethodAllowed(readOnly, "POST") {
		t.Fatal("POST should be rejected")
	}

	writeOnly := &listener.ListenCfg{AllowMethods: []string{"POST"}}
	if MethodAllowed(writeOnly, "HEAD") {
		t.Fatal("HEAD must not follow POST-only rules")
	}

	if !MethodAllowed(nil, "GET") {
		t.Fatal("nil config should not restrict")
	}
}
