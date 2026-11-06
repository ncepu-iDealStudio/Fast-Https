package config

import "testing"

func TestServerNames(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"单域名", "example.com", []string{"example.com"}},
		{"多域名空格分隔", "a.example b.example", []string{"a.example", "b.example"}},
		{"多域名多余空白", "  a.example   b.example  ", []string{"a.example", "b.example"}},
		{"三个域名", "a.com b.com c.com", []string{"a.com", "b.com", "c.com"}},
		{"空字符串", "", nil},
		{"纯空白", "   ", nil},
		{"IP 地址", "192.168.1.10", []string{"192.168.1.10"}},
		{"localhost", "localhost", []string{"localhost"}},
		{"混合域名与 localhost", "example.com localhost", []string{"example.com", "localhost"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Server{ServerName: c.in}
			got := s.ServerNames()
			if len(got) != len(c.want) {
				t.Fatalf("ServerNames(%q) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("ServerNames(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestPrimaryServerName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"单域名", "example.com", "example.com"},
		{"多域名取第一个", "a.example b.example", "a.example"},
		{"空字符串", "", ""},
		{"纯空白", "   ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Server{ServerName: c.in}
			if got := s.PrimaryServerName(); got != c.want {
				t.Errorf("PrimaryServerName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
