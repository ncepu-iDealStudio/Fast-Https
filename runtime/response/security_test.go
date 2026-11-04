package response

import (
	"fast-https/config"
	"strings"
	"testing"
)

func withSecurity(t *testing.T, sec *config.SecurityConfig) {
	t.Helper()
	previous := *config.Current()
	previousMIME := config.MIME()
	t.Cleanup(func() { config.Store(&previous, previousMIME) })

	snapshot := &config.Fast_Https{Security: sec}
	config.Store(snapshot, previousMIME)
}

func TestApplySecurityHeadersDefaults(t *testing.T) {
	withSecurity(t, nil)

	res := ResponseInit()
	res.SetFirstLine(200, "OK")
	ApplySecurityHeaders(res)

	if got := res.Headers["X-Content-Type-Options"]; got != "nosniff" {
		t.Fatalf("nosniff = %q", got)
	}
	if res.Headers["Strict-Transport-Security"] != "" {
		t.Fatal("HSTS must stay off unless configured")
	}

	// 幂等：再调一次不会重复或覆盖。
	ApplySecurityHeaders(res)
	if strings.Count(res.Headers["X-Content-Type-Options"], "nosniff") != 1 {
		t.Fatal("headers should be applied once")
	}
}

func TestApplySecurityHeadersConfigured(t *testing.T) {
	withSecurity(t, &config.SecurityConfig{
		HostCheck: config.HostCheckWarn,
		Headers: config.SecurityHeaders{
			HSTS:           "max-age=31536000",
			FrameOptions:   "DENY",
			ReferrerPolicy: "no-referrer",
			CSP:            "default-src 'self'",
		},
	})

	res := ResponseInit()
	ApplySecurityHeaders(res)

	for key, want := range map[string]string{
		"Strict-Transport-Security": "max-age=31536000",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Content-Security-Policy":   "default-src 'self'",
	} {
		if got := res.Headers[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestApplySecurityHeadersKeepsModuleValue(t *testing.T) {
	withSecurity(t, &config.SecurityConfig{
		Headers: config.SecurityHeaders{FrameOptions: "DENY"},
	})

	res := ResponseInit()
	res.SetHeader("X-Frame-Options", "SAMEORIGIN")
	ApplySecurityHeaders(res)

	if got := res.Headers["X-Frame-Options"]; got != "SAMEORIGIN" {
		t.Fatalf("module value overwritten: %q", got)
	}
}

func TestDefaultStatusResponses(t *testing.T) {
	res := DefaultMethodNotAllowed([]string{"GET", "POST"})
	if res.FirstLine != "HTTP/1.1 405 METHODNOTALLOWED" {
		t.Fatalf("first line = %q", res.FirstLine)
	}
	if res.Headers["Allow"] != "GET, POST" {
		t.Fatalf("Allow = %q", res.Headers["Allow"])
	}

	res = DefaultMisdirected()
	if res.FirstLine != "HTTP/1.1 421 MISDIRECTEDREQUEST" {
		t.Fatalf("first line = %q", res.FirstLine)
	}
}
