package packaging

import (
	"encoding/json"
	"testing"
)

// TestDefaultConfigShipsSharedCacheZone 锁住随包主配置预置的缓存区：
// 应是一个可共用的 default_cache（1 GiB 软上限），而不是测试用的小容量。
func TestDefaultConfigShipsSharedCacheZone(t *testing.T) {
	raw := readPkgFile(t, "config/fast-https.json")
	var cfg struct {
		Cache []struct {
			Name    string   `json:"name"`
			Path    string   `json:"path"`
			Valid   []string `json:"valid"`
			MaxSize int64    `json:"max_size"`
		} `json:"cache"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("解析 config/fast-https.json: %v", err)
	}
	if len(cfg.Cache) != 1 {
		t.Fatalf("随包主配置应只有一个共享缓存区，实际 %d 个", len(cfg.Cache))
	}
	z := cfg.Cache[0]
	if z.Name != "default_cache" {
		t.Errorf("缓存区名 = %q，应为 default_cache", z.Name)
	}
	if z.Path == "" || len(z.Valid) == 0 {
		t.Errorf("缓存区缺少目录或有效期：%+v", z)
	}
	if z.MaxSize != 1<<30 {
		t.Errorf("max_size = %d，应为 1 GiB 软上限", z.MaxSize)
	}
}
