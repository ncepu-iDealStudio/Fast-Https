package proxy

import "testing"

// proxy_pass 带 URI 时按 nginx 规则替换 location 前缀；不带 URI 原样转发。
func TestUpstreamTarget(t *testing.T) {
	cases := []struct {
		name string
		path string
		loc  []int
		pass string
		want string
	}{
		{"no uri keeps path", "/api/users?x=1", []int{0, 4}, "", "/api/users?x=1"},
		{"slash replaces prefix", "/api/users?x=1", []int{0, 4}, "/", "/users?x=1"},
		{"uri without trailing slash", "/api/users", []int{0, 4}, "/v1", "/v1/users"},
		{"uri with trailing slash", "/api/users", []int{0, 5}, "/v1/", "/v1/users"},
		{"exact match", "/api", []int{0, 4}, "/v1/", "/v1/"},
		{"exact match with query", "/api?x=1", []int{0, 4}, "/v1", "/v1?x=1"},
		{"query only", "/api?x=1", []int{0, 4}, "/", "/?x=1"},
		{"not at start keeps path", "/x/api", []int{2, 6}, "/v1", "/x/api"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := upstreamTarget(c.path, c.loc, c.pass); got != c.want {
				t.Fatalf("upstreamTarget(%q, %v, %q) = %q, want %q", c.path, c.loc, c.pass, got, c.want)
			}
		})
	}
}
