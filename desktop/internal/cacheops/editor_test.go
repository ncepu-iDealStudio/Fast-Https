package cacheops

import (
	"strings"
	"testing"

	"fast-https-desktop/internal/model"
)

// parseZones 把 cache list 的 JSON 映射成桌面端模型。
func TestParseZones(t *testing.T) {
	data := map[string]any{
		"zones": []any{
			map[string]any{
				"name": "edge", "path": "./cache/edge",
				"valid": []any{"200:10m", "any:30s"}, "max_size": float64(1024),
			},
		},
	}
	zones := parseZones(data)
	if len(zones) != 1 {
		t.Fatalf("zones = %+v", zones)
	}
	z := zones[0]
	if z.Name != "edge" || z.Path != "./cache/edge" || z.MaxSize != 1024 {
		t.Fatalf("zone = %+v", z)
	}
	if len(z.Valid) != 2 || z.Valid[0] != "200:10m" {
		t.Fatalf("valid = %v", z.Valid)
	}
	if got := parseZones(map[string]any{}); len(got) != 0 {
		t.Fatalf("empty data = %+v", got)
	}
}

// validate 是写操作前的快速检查，权威校验仍在 CLI。
func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		zone model.CacheZone
		want string
	}{
		{"missing name", model.CacheZone{Path: "./cache", Valid: []string{"any:10s"}}, "缓存区名"},
		{"missing path", model.CacheZone{Name: "edge", Valid: []string{"any:10s"}}, "目录"},
		{"negative size", model.CacheZone{Name: "edge", Path: "./cache", Valid: []string{"any:10s"}, MaxSize: -1}, "负数"},
		{"missing valid", model.CacheZone{Name: "edge", Path: "./cache", Valid: []string{" "}}, "有效期"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validate(c.zone)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want contains %q", err, c.want)
			}
		})
	}
	ok := model.CacheZone{Name: "edge", Path: "./cache", Valid: []string{"any:10s"}}
	if err := validate(ok); err != nil {
		t.Fatalf("valid zone rejected: %v", err)
	}
}

// zoneArgs 把有效期拼成 --valid 参数；空列表显式传空值，update 才能清空。
func TestZoneArgs(t *testing.T) {
	args := zoneArgs(model.CacheZone{Valid: []string{"200:10m", "any:30s"}})
	if strings.Join(args, " ") != "--valid 200:10m --valid any:30s" {
		t.Fatalf("args = %v", args)
	}
	if off := zoneArgs(model.CacheZone{}); len(off) != 1 || off[0] != "--valid=" {
		t.Fatalf("empty args = %v", off)
	}
}
