package filters

import (
	"fast-https/config"
	"fast-https/runtime/listener"
	"regexp"
	"testing"
)

// TestMultiDomainHostMap 验证多域名 server_name 的 HostMap 注册：
// 每个域名（主域名 + 别名）都映射到同一组 locations。
func TestMultiDomainHostMap(t *testing.T) {
	re := regexp.MustCompile("/")
	cfg1 := listener.ListenCfg{
		ID: 0, ServerName: "a.example:80", Path: "/", PathRe: re, Type: config.LOCAL,
		Aliases: []string{"b.example:80", "c.example:80"},
	}
	cfg2 := listener.ListenCfg{
		ID: 1, ServerName: "other.example:80", Path: "/", PathRe: re, Type: config.LOCAL,
	}

	lis := &listener.Listener{
		Port: "80",
		Cfg:  []listener.ListenCfg{cfg1, cfg2},
		HostMap: map[string][]listener.ListenCfg{
			"a.example:80":     {cfg1},
			"b.example:80":     {cfg1}, // 别名指向同一组
			"c.example:80":     {cfg1}, // 别名指向同一组
			"other.example:80": {cfg2},
		},
	}

	// 主域名命中
	hosts := lis.Hosts()
	if cfgs := hosts["a.example:80"]; len(cfgs) != 1 || cfgs[0].ID != 0 {
		t.Fatalf("主域名 a.example:80 命中 %v", cfgs)
	}
	// 别名命中同一组
	if cfgs := hosts["b.example:80"]; len(cfgs) != 1 || cfgs[0].ID != 0 {
		t.Fatalf("别名 b.example:80 命中 %v", cfgs)
	}
	if cfgs := hosts["c.example:80"]; len(cfgs) != 1 || cfgs[0].ID != 0 {
		t.Fatalf("别名 c.example:80 命中 %v", cfgs)
	}
	// 其他 server 块不受影响
	if cfgs := hosts["other.example:80"]; len(cfgs) != 1 || cfgs[0].ID != 1 {
		t.Fatalf("other.example:80 命中 %v", cfgs)
	}
	// 未注册域名不命中
	if cfgs := hosts["unknown.example:80"]; len(cfgs) != 0 {
		t.Fatalf("unknown.example:80 不应命中，实际 %v", cfgs)
	}
}

// TestMultiDomainDefaultHosts 验证多域名时 DefaultHosts 仍用主域名。
func TestMultiDomainDefaultHosts(t *testing.T) {
	re := regexp.MustCompile("/")
	cfg := listener.ListenCfg{
		ID: 0, ServerName: "a.example:80", Path: "/", PathRe: re, Type: config.LOCAL,
		Aliases: []string{"b.example:80"},
	}
	lis := &listener.Listener{
		Port: "80",
		Cfg:  []listener.ListenCfg{cfg},
		HostMap: map[string][]listener.ListenCfg{
			"a.example:80": {cfg},
			"b.example:80": {cfg},
		},
	}
	hosts := lis.DefaultHosts()
	if len(hosts) != 1 || hosts[0].ServerName != "a.example:80" {
		t.Fatalf("DefaultHosts 应返回主域名 a.example:80，实际 %v", hosts)
	}
}

// TestMultiDomainExtraCerts 验证多域名时 ExtraCerts 被正确填充。
func TestMultiDomainExtraCerts(t *testing.T) {
	cfg := listener.ListenCfg{
		ServerName: "a.example:443",
		SSL:        listener.SSLkv{SslKey: "a.pem", SslValue: "a-key.pem"},
		ExtraCerts: []listener.SSLkv{
			{SslKey: "b.pem", SslValue: "b-key.pem"},
			{SslKey: "c.pem", SslValue: "c-key.pem"},
		},
	}
	if len(cfg.ExtraCerts) != 2 {
		t.Fatalf("ExtraCerts 应有 2 个，实际 %d", len(cfg.ExtraCerts))
	}
	if cfg.ExtraCerts[0].SslKey != "b.pem" {
		t.Errorf("ExtraCerts[0].SslKey = %q, want b.pem", cfg.ExtraCerts[0].SslKey)
	}
	if cfg.ExtraCerts[1].SslKey != "c.pem" {
		t.Errorf("ExtraCerts[1].SslKey = %q, want c.pem", cfg.ExtraCerts[1].SslKey)
	}
}
