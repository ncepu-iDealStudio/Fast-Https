package config

import (
	"path/filepath"
	"testing"
)

// TestReloadInvalidKeepsPreviousConfig 覆盖「reload 失败不能把服务带进半死
// 状态」：新配置非法时，运行时快照必须保持 reload 之前的样子。
//
// ReloadFile 是「先校验、再解析、最后 Store」的顺序，任何一步失败都不该
// 触碰 Current()。这条用例把那个顺序钉死——一旦有人改成先清空再加载，
// 正在服务的实例会拿着空配置继续跑。
func TestReloadInvalidKeepsPreviousConfig(t *testing.T) {
	prev := Current()
	t.Cleanup(func() { Store(prev, map[string]string{}) })

	const marker = "before-reload"
	Store(&Fast_Https{LogRoot: marker}, map[string]string{})

	path := filepath.Join("testdata", "invalid-listen.json")
	if err := ReloadFile(path); err == nil {
		t.Fatal("listen 非法的配置应当校验失败")
	}

	got := Current()
	if got == nil {
		t.Fatal("reload 失败后 Current() 变成 nil")
	}
	if got.LogRoot != marker {
		t.Fatalf("reload 失败后旧配置被改动：LogRoot=%q，期望 %q", got.LogRoot, marker)
	}
}

// TestReloadMissingFileKeepsPreviousConfig 覆盖「配置文件不存在」这条路径：
// 同样不能把快照清空。
func TestReloadMissingFileKeepsPreviousConfig(t *testing.T) {
	prev := Current()
	t.Cleanup(func() { Store(prev, map[string]string{}) })

	const marker = "before-missing"
	Store(&Fast_Https{LogRoot: marker}, map[string]string{})

	if err := ReloadFile(filepath.Join("testdata", "does-not-exist.json")); err == nil {
		t.Fatal("不存在的配置应当报错")
	}
	if got := Current(); got == nil || got.LogRoot != marker {
		t.Fatalf("reload 失败后旧配置被改动：%+v", got)
	}
}
