package cache

import (
	"encoding/gob"
	"fast-https/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shardPath 复现 WriteCache 的目录分片布局（md5 尾两位/一位两层）。
func shardPath(dir, key string) string {
	return filepath.Join(dir, key[len(key)-1:], key[len(key)-3:len(key)-1])
}

func writeDiskEntry(t *testing.T, dir, key string, expire int, legacy bool) {
	t.Helper()
	node := CacheNode{Expire: expire, Valid: 60, Md5: key, Path: shardPath(dir, key)}
	if err := os.MkdirAll(node.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(node.Path, key), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if legacy {
		// 旧格式：只有 Head/Size/Data，没有状态行与头部
		if err := gob.NewEncoder(file).Encode(struct {
			Head CacheHead
			Size int
			Data []byte
		}{Head: CacheHead(node), Size: 3, Data: []byte("old")}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := gob.NewEncoder(file).Encode(CacheEntry{
		Head:      CacheHead(node),
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Headers:   map[string]string{"Content-Type": "text/html"},
		Data:      []byte("recovered"),
		Size:      9,
	}); err != nil {
		t.Fatal(err)
	}
}

// withCacheDir 把 config.Current().Cache 指向 dir，返回恢复函数。
func withCacheDir(t *testing.T, dir string) {
	t.Helper()
	snap := config.Current().Cache
	config.Current().Cache = []config.CacheZone{{Name: "test", Path: dir, Valid: []string{"10s"}}}
	t.Cleanup(func() { config.Current().Cache = snap })
}

// 磁盘恢复矩阵：可恢复条目重建索引并可回放；损坏 gob / 旧格式 /
// 已过期条目一律不进索引，且过期文件被顺带清理。
func TestLoadCacheRecoversIndexFromDisk(t *testing.T) {
	dir := t.TempDir()
	now := int(time.Now().Unix())

	validKey := GetMd5("load-valid")
	writeDiskEntry(t, dir, validKey, now+3600, false)

	legacyKey := GetMd5("load-legacy")
	writeDiskEntry(t, dir, legacyKey, now+3600, true)

	if err := os.WriteFile(filepath.Join(dir, "corrupt-entry"), []byte("this is not gob"), 0o644); err != nil {
		t.Fatal(err)
	}

	expiredKey := GetMd5("load-expired")
	writeDiskEntry(t, dir, expiredKey, now-10, false)
	expiredFile := filepath.Join(shardPath(dir, expiredKey), expiredKey)

	withCacheDir(t, dir)
	cc := NewCacheContainer() // 独立容器，不污染 GCacheContainer
	cc.LoadCache()

	entry, ok := cc.ReadCache(validKey)
	if !ok {
		t.Fatal("valid entry was not recovered into index")
	}
	if string(entry.Data) != "recovered" || entry.FirstLine != "HTTP/1.1 200 OK" {
		t.Fatalf("recovered entry = %+v", entry)
	}

	for name, key := range map[string]string{
		"legacy gob":  legacyKey,
		"corrupt gob": "corrupt-entry",
		"expired":     expiredKey,
	} {
		if _, ok := cc.Index.Touch(key); ok {
			t.Errorf("%s must not enter the index", name)
		}
	}
	if _, err := os.Stat(expiredFile); !os.IsNotExist(err) {
		t.Errorf("expired file must be removed on recovery, got %v", err)
	}
}

// 扫描目标不是目录（配置错误/文件被替换）：跳过且不 panic
func TestLoadCacheToleratesUnreadableDir(t *testing.T) {
	notDir := filepath.Join(t.TempDir(), "plain-file")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCacheDir(t, notDir)

	cc := NewCacheContainer()
	cc.LoadCache() // 不得 panic
	if cc.Index.Len() != 0 {
		t.Fatalf("index = %d entries, want 0", cc.Index.Len())
	}
}

// 空目录与不存在的目录：正常空恢复
func TestLoadCacheEmptyAndMissingDir(t *testing.T) {
	for _, dir := range []string{t.TempDir(), filepath.Join(t.TempDir(), "not-exists")} {
		withCacheDir(t, dir)
		cc := NewCacheContainer()
		cc.LoadCache()
		if cc.Index.Len() != 0 {
			t.Fatalf("index = %d entries, want 0", cc.Index.Len())
		}
	}
}

// EnsureDirs 让配置里每个缓存区目录在磁盘上存在（含多级路径）。
// 缓存条目是懒写入的，目录本身属于缓存区定义，不应等首条响应被缓存后才出现。
func TestEnsureDirsCreatesZonePaths(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".cache", "edge")
	withCacheDir(t, dir)

	NewCacheContainer().EnsureDirs()

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("缓存区目录未创建: %v", err)
	}
}
