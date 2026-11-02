package config

import (
	"strings"
	"testing"
)

// auth 段：只支持 basic，且 user/pswd 必须写全，realm 不能破坏响应头。
func TestValidateLocationAuth(t *testing.T) {
	cases := []struct {
		name    string
		auth    PathAuth
		wantErr string
	}{
		{name: "absent", auth: PathAuth{}},
		{name: "complete basic", auth: PathAuth{AuthType: "basic", User: "alice", Pswd: "s3cret"}},
		{name: "scheme is case insensitive", auth: PathAuth{AuthType: "BASIC", User: "alice", Pswd: "s3cret"}},
		{name: "realm is allowed", auth: PathAuth{AuthType: "basic", User: "alice", Pswd: "s3cret", Realm: "Private"}},
		{name: "unsupported type", auth: PathAuth{AuthType: "digest", User: "alice", Pswd: "s3cret"}, wantErr: "must be basic"},
		{name: "missing user", auth: PathAuth{AuthType: "basic", Pswd: "s3cret"}, wantErr: "auth.user is required"},
		{name: "missing pswd", auth: PathAuth{AuthType: "basic", User: "alice"}, wantErr: "auth.pswd is required"},
		{name: "quoted realm", auth: PathAuth{AuthType: "basic", User: "alice", Pswd: "s3cret", Realm: `a"b`}, wantErr: "auth.realm"},
		{name: "realm with newline", auth: PathAuth{AuthType: "basic", User: "alice", Pswd: "s3cret", Realm: "a\nb"}, wantErr: "auth.realm"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateLocationAuth(&c.auth, "http.server[0]", 0)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

// validateServer 必须真正调用 auth 校验，否则只写 type 的站点仍可保存。
func TestValidateServerRejectsIncompleteAuth(t *testing.T) {
	server := jsonServer{
		Listen:     flexList{"80"},
		ServerName: "local",
		Locations: []jsonPath{{
			URL:  "/admin",
			Type: "local",
			Root: ".",
			Auth: PathAuth{AuthType: "basic", User: "alice"},
		}},
	}

	err := validateServer(&server, ".", ".", "site", false)
	if err == nil || !strings.Contains(err.Error(), "auth.pswd is required") {
		t.Fatalf("error = %v, want incomplete auth rejected", err)
	}
}
