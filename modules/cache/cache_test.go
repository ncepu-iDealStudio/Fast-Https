package cache

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteThenReadReturnsReplayableEntry(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Headers:   map[string]string{"Content-Type": "text/html"},
		Data:      []byte("hello"),
	})

	entry := waitEntry(t, key)
	if string(entry.Data) != "hello" {
		t.Errorf("Data = %q, want hello", entry.Data)
	}
	if entry.Status != 200 {
		t.Errorf("Status = %d, want 200", entry.Status)
	}
	if entry.FirstLine != "HTTP/1.1 200 OK" {
		t.Errorf("FirstLine = %q", entry.FirstLine)
	}
	if entry.Headers["Content-Type"] != "text/html" {
		t.Errorf("Headers = %v", entry.Headers)
	}
}

func TestReadRefreshesExpire(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{FirstLine: "HTTP/1.1 200 OK", Status: 200, Data: []byte("x")})
	waitEntry(t, key)

	node, ok := GCacheContainer.Index.Touch(key)
	if !ok {
		t.Fatal("index entry missing")
	}
	node.Expire = 1

	if _, ok := GCacheContainer.ReadCache(key); !ok {
		t.Fatal("expected a cache hit")
	}
	refreshed, _ := GCacheContainer.Index.Touch(key)
	if refreshed.Expire <= 1 {
		t.Errorf("Expire = %d, want it refreshed past 1", refreshed.Expire)
	}
}

func TestExpireCacheRemovesExpiredEntry(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{FirstLine: "HTTP/1.1 200 OK", Status: 200, Data: []byte("x")})
	entry := waitEntry(t, key)
	_ = entry

	node, ok := GCacheContainer.Index.Touch(key)
	if !ok {
		t.Fatal("index entry missing")
	}
	realPath := filepath.Join(node.Path, key)
	node.Expire = 0

	GCacheContainer.ExpireCache()

	if _, ok := GCacheContainer.ReadCache(key); ok {
		t.Error("expired entry must not be readable")
	}
	if _, err := os.Stat(realPath); !os.IsNotExist(err) {
		t.Errorf("expired file %s should be removed, got %v", realPath, err)
	}
}

func TestWriteCacheRespectsMaxSize(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 4, dir, CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Data:      []byte("0123456789"),
	})

	if _, ok := GCacheContainer.ReadCache(key); ok {
		t.Error("entry over max_size must not be cached")
	}
	GCacheContainer.mu.Lock()
	size, tracked := GCacheContainer.sizes[key]
	GCacheContainer.mu.Unlock()
	if tracked {
		t.Errorf("rejected write must not be accounted, got %d", size)
	}
}

func TestReadCacheMissesWhenFileIsGone(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{FirstLine: "HTTP/1.1 200 OK", Status: 200, Data: []byte("x")})
	waitEntry(t, key)

	node, _ := GCacheContainer.Index.Touch(key)
	if err := os.Remove(filepath.Join(node.Path, key)); err != nil {
		t.Fatal(err)
	}

	if _, ok := GCacheContainer.ReadCache(key); ok {
		t.Error("missing file must be a miss")
	}
	if _, ok := GCacheContainer.Index.Touch(key); ok {
		t.Error("index entry should be dropped after a miss")
	}
}

func TestReadCacheRejectsLegacyEntry(t *testing.T) {
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer discard(key)

	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{FirstLine: "HTTP/1.1 200 OK", Status: 200, Data: []byte("x")})
	waitEntry(t, key)

	node, _ := GCacheContainer.Index.Touch(key)
	realPath := filepath.Join(node.Path, key)

	// 旧格式条目只有 Head/Size/Data，没有状态行与头部，无法回放。
	legacy := struct {
		Head CacheHead
		Size int
		Data []byte
	}{Head: CacheHead(*node), Size: 3, Data: []byte("old")}
	file, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := gob.NewEncoder(file).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	if _, ok := GCacheContainer.ReadCache(key); ok {
		t.Error("legacy entry must be treated as a miss")
	}
}

// discard 清理测试写入的索引与容量记账。
func discard(key string) {
	GCacheContainer.Index.Remove(key)
	GCacheContainer.release(key)
}

// waitEntry 轮询等待异步落盘完成。
func waitEntry(t *testing.T, key string) *CacheEntry {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if entry, ok := GCacheContainer.ReadCache(key); ok {
			return entry
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("cache entry %s was never written", key)
	return nil
}
