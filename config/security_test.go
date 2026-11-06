package config

import (
	"strings"
	"testing"
)

func TestSecurityDefaults(t *testing.T) {
	var nilSec *SecurityConfig
	if nilSec.HostCheckMode() != HostCheckOff {
		t.Fatalf("nil security should default to off")
	}
	if !nilSec.NoSniffEnabled() {
		t.Fatal("nosniff should be on without a security section")
	}

	sec := &SecurityConfig{}
	sec.applyDefaults()
	if sec.HostCheck != HostCheckOff {
		t.Fatalf("host_check = %q, want off", sec.HostCheck)
	}
	if sec.Headers.NoSniff == nil || !*sec.Headers.NoSniff {
		t.Fatal("nosniff should default to true")
	}

	disabled := false
	explicit := &SecurityConfig{Headers: SecurityHeaders{NoSniff: &disabled}}
	if explicit.NoSniffEnabled() {
		t.Fatal("explicit false must win")
	}
}

func TestSecurityValidate(t *testing.T) {
	for _, mode := range []string{HostCheckOff, HostCheckWarn, HostCheckEnforce, ""} {
		sec := &SecurityConfig{HostCheck: mode}
		if err := sec.validate(); err != nil {
			t.Fatalf("mode %q should be accepted: %v", mode, err)
		}
	}
	if err := (&SecurityConfig{HostCheck: "strict"}).validate(); err == nil {
		t.Fatal("unknown host_check should be rejected")
	} else if !strings.Contains(err.Error(), "host_check") {
		t.Fatalf("error should name the field: %v", err)
	}
}

func TestAllowMethodsNormalizedAndValidated(t *testing.T) {
	path := jsonPath{URL: "/", Type: "local", Root: "./httpdoc/root",
		AllowMethods: []string{" get ", "Post", ""}}
	got := normalizeMethods(path.AllowMethods)
	if len(got) != 2 || got[0] != "GET" || got[1] != "POST" {
		t.Fatalf("normalizeMethods = %v", got)
	}

	server := jsonServer{
		Listen:     flexList{"80"},
		ServerName: "a.example",
		Locations:  []jsonPath{{URL: "/", Type: "local", Root: "./httpdoc/root", AllowMethods: []string{" "}}},
	}
	if err := validateServer(&server, ".", ".", "site", false); err == nil {
		t.Fatal("empty method entry should be rejected")
	}

	server.Locations[0].AllowMethods = []string{"GET POST"}
	if err := validateServer(&server, ".", ".", "site", false); err == nil {
		t.Fatal("method list must not contain two tokens")
	}
}
