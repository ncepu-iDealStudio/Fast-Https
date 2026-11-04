package filters

import (
	"regexp"
	"testing"

	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
)

func locationCfg(path string) listener.ListenCfg {
	return listener.ListenCfg{Path: path, PathRe: regexp.MustCompile(path)}
}

func matchPath(t *testing.T, path string, hosts []listener.ListenCfg) (*listener.ListenCfg, *core.Event) {
	t.Helper()
	ev := &core.Event{RR: core.RRcircle{Req: request.RequestInit(false)}}
	ev.RR.Req.Path = path
	cfg, ok := matchLocation(ev, hosts)
	if !ok {
		return nil, ev
	}
	return cfg, ev
}

// `/`（静态）+ `/api`（反代）时，`/api/x` 必须命中反代规则，
// 不能因为 `/` 写在前面被正则包含匹配吃掉。
func TestMatchLocationPrefersLongestPrefix(t *testing.T) {
	hosts := []listener.ListenCfg{
		locationCfg("/"),
		locationCfg("/api"),
	}

	cfg, ev := matchPath(t, "/api/users", hosts)
	if cfg == nil || cfg.Path != "/api" {
		t.Fatalf("matched %v, want /api", cfg)
	}
	if ev.RR.OriginPath != "/users" {
		t.Fatalf("OriginPath = %q, want /users", ev.RR.OriginPath)
	}

	cfg, ev = matchPath(t, "/index.html", hosts)
	if cfg == nil || cfg.Path != "/" {
		t.Fatalf("matched %v, want /", cfg)
	}
	if ev.RR.OriginPath != "index.html" {
		t.Fatalf("OriginPath = %q", ev.RR.OriginPath)
	}
}

// 前缀写反了顺序也一样：更长的前缀优先，与配置顺序无关。
func TestMatchLocationOrderIndependent(t *testing.T) {
	hosts := []listener.ListenCfg{
		locationCfg("/api/v2"),
		locationCfg("/api"),
		locationCfg("/"),
	}
	cfg, _ := matchPath(t, "/api/v2/users", hosts)
	if cfg == nil || cfg.Path != "/api/v2" {
		t.Fatalf("matched %v, want /api/v2", cfg)
	}
}

// 同样的前缀：保持配置顺序，先写的生效。
func TestMatchLocationTieKeepsConfigOrder(t *testing.T) {
	first := locationCfg("/same")
	first.ServerName = "first:80"
	second := locationCfg("/same")
	second.ServerName = "second:80"
	hosts := []listener.ListenCfg{first, second}

	cfg, _ := matchPath(t, "/same/x", hosts)
	if cfg == nil || cfg.ServerName != "first:80" {
		t.Fatalf("matched %v, want the first configured rule", cfg)
	}
}

// 从起点命中的规则优于中途命中的正则，哪怕后者更长。
func TestMatchLocationStartWinsOverMidPath(t *testing.T) {
	hosts := []listener.ListenCfg{
		locationCfg(`\.php$`),
		locationCfg("/"),
	}
	cfg, _ := matchPath(t, "/a.php", hosts)
	if cfg == nil || cfg.Path == `\.php$` {
		t.Fatalf("mid-path regex should lose to the prefix rule, got %v", cfg)
	}
}

// 一条都不命中时返回 false，调用方按默认 404 处理。
func TestMatchLocationNoMatch(t *testing.T) {
	if cfg, _ := matchPath(t, "/nothing", []listener.ListenCfg{locationCfg("/api")}); cfg != nil {
		t.Fatalf("expected no match, got %v", cfg)
	}
}

// 非标准端口上客户端不会把 :port 写进 Host，但 HostMap 的键是 name:port：
// 必须能落到正确的 server 块，而不是 default_server。
func TestGFilterHostPathMatchesHostWithPortOnCustomPort(t *testing.T) {
	lis := &listener.Listener{
		Port: "18080",
		HostMap: map[string][]listener.ListenCfg{
			"first.local:18080": {{ServerName: "first.local:18080", Path: "/", PathRe: regexp.MustCompile("/")}},
			"site.local:18080": {
				{ServerName: "site.local:18080", Path: "/", PathRe: regexp.MustCompile("/")},
				{ServerName: "site.local:18080", Path: "/api", PathRe: regexp.MustCompile("/api")},
			},
		},
	}
	ev := core.NewEvent(lis, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Headers["Host"] = "site.local"
	ev.RR.Req.Path = "/api/users"

	cfg, ok := GFilterHostPath(ev)
	if !ok || cfg.ServerName != "site.local:18080" || cfg.Path != "/api" {
		t.Fatalf("matched %v, ok=%v; want site.local:18080 /api", cfg, ok)
	}
}

// 带端口的 Host 直接命中，不该被拼接逻辑带偏。
func TestHostCfgsKeepsExplicitPort(t *testing.T) {
	lis := &listener.Listener{
		Port: "18080",
		HostMap: map[string][]listener.ListenCfg{
			"site.local:18080": {{Path: "/"}},
		},
	}
	ev := core.NewEvent(lis, nil)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Headers["Host"] = "site.local:18080"

	if cfgs := hostCfgs(ev); len(cfgs) != 1 {
		t.Fatalf("cfgs = %v", cfgs)
	}

	// 未知 Host 不拼端口乱命中
	ev.RR.Req.Headers["Host"] = "other.local"
	if cfgs := hostCfgs(ev); len(cfgs) != 0 {
		t.Fatalf("unknown host matched %v", cfgs)
	}
}
