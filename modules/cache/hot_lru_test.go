package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestEntry(key, body string) *CacheEntry {
	return &CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Headers:   map[string]string{"Content-Type": "text/html"},
		Data:      []byte(body),
	}
}

func seedDiskEntry(t *testing.T, cc *CacheContainer, key, body string) {
	t.Helper()
	node := CacheNode{
		Expire: int(time.Now().Unix()) + 60,
		Valid:  60,
		Md5:    key,
		Path:   filepath.Join(t.TempDir(), key[len(key)-1:]),
	}
	if err := writeEntryToDisk(CacheEntry{Head: CacheHead(node), FirstLine: "HTTP/1.1 200 OK", Data: []byte(body)}); err != nil {
		t.Fatal(err)
	}
	cc.Index.Set(&node)
}

// 热层命中后即使磁盘文件被删，仍能读到内容（加速层语义）
func TestHotLayerServesWithoutDisk(t *testing.T) {
	cc := newCacheContainerWithCaps(16, 1<<20)
	key := "abcdef"
	seedDiskEntry(t, cc, key, "hello-hot")

	if _, ok := cc.ReadCache(key); !ok {
		t.Fatal("first read (disk fill) should hit")
	}
	// 删掉磁盘文件模拟"只在热层"
	matches, _ := filepath.Glob(filepath.Join(cc.hot[key].entry.Head.Path, "*"))
	_ = matches
	node, _ := cc.Index.Touch(key)
	_ = os.Remove(filepath.Join(node.Path, key))

	got, ok := cc.ReadCache(key)
	if !ok {
		t.Fatal("hot layer should serve after disk removal")
	}
	if string(got.Data) != "hello-hot" {
		t.Fatalf("hot body = %q", got.Data)
	}
}

// 超过条数上限时按 LRU 淘汰最久未用的
func TestHotLayerLRUEviction(t *testing.T) {
	cc := newCacheContainerWithCaps(2, 1<<20)

	cc.hotPut("a", newTestEntry("a", "1"))
	cc.hotPut("b", newTestEntry("b", "2"))
	cc.hotGet("a") // a 变为最近使用
	cc.hotPut("c", newTestEntry("c", "3"))

	cc.hotMu.Lock()
	_, bAlive := cc.hot["b"]
	cc.hotMu.Unlock()
	if bAlive {
		t.Fatal("least recently used entry b should be evicted")
	}
	if cc.hotGet("a") == nil || cc.hotGet("c") == nil {
		t.Fatal("recently used entries should survive")
	}
}

// 字节上限同样触发淘汰
func TestHotLayerByteCapEviction(t *testing.T) {
	cc := newCacheContainerWithCaps(100, 32)
	cc.hotPut("big", newTestEntry("big", string(make([]byte, 128))))
	if cc.hotGet("big") != nil {
		t.Fatal("entry exceeding byte cap should be evicted immediately")
	}
}

// 过期清理同时摘除热层
func TestExpireRemovesHotEntry(t *testing.T) {
	cc := newCacheContainerWithCaps(16, 1<<20)
	key := "expireme"
	seedDiskEntry(t, cc, key, "soon-gone")
	if _, ok := cc.ReadCache(key); !ok {
		t.Fatal("seed read should hit")
	}

	// 把索引节点的过期时间改到过去
	node, _ := cc.Index.Touch(key)
	node.Expire = int(time.Now().Unix()) - 1

	cc.ExpireCache()
	if cc.hotGet(key) != nil {
		t.Fatal("expired entry should be removed from hot layer")
	}
	if _, ok := cc.ReadCache(key); ok {
		t.Fatal("expired entry should not be readable")
	}
}
