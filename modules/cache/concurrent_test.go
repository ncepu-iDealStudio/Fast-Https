package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// 并发写读清理：-race 下不得出现竞争，且每个写入的键都必须真正进索引、
// 落盘、可回放，容量记账不能重复累加。
func TestConcurrentWriteReadExpireKeepsEveryEntry(t *testing.T) {
	const (
		writers   = 8
		perWriter = 12
		readers   = 4
	)

	type record struct {
		key  string
		body string
	}
	records := make([]record, 0, writers*perWriter)
	for g := 0; g < writers; g++ {
		for i := 0; i < perWriter; i++ {
			records = append(records, record{
				key:  GetMd5(fmt.Sprintf("%s/%d/%d", t.Name(), g, i)),
				body: fmt.Sprintf("body-%d-%d", g, i),
			})
		}
	}
	defer func() {
		for _, rec := range records {
			GCacheContainer.Index.Remove(rec.key)
			GCacheContainer.release(rec.key)
			GCacheContainer.hotRemove(rec.key)
		}
	}()

	dir := t.TempDir()
	stop := make(chan struct{})
	var background sync.WaitGroup

	// 读者不停地读（多数是未命中，写入后才命中），把续期与热层放进竞争窗口。
	for r := 0; r < readers; r++ {
		background.Add(1)
		go func() {
			defer background.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for _, rec := range records {
						GCacheContainer.ReadCache(rec.key)
					}
				}
			}
		}()
	}

	// 清理协程与写入并发：此刻条目都未过期，ExpireCache 只做索引遍历。
	background.Add(1)
	go func() {
		defer background.Done()
		for {
			select {
			case <-stop:
				return
			default:
				GCacheContainer.ExpireCache()
			}
		}
	}()

	var writersDone sync.WaitGroup
	for g := 0; g < writers; g++ {
		writersDone.Add(1)
		go func(g int) {
			defer writersDone.Done()
			for i := 0; i < perWriter; i++ {
				rec := records[g*perWriter+i]
				GCacheContainer.WriteCache(rec.key, 60, 0, dir, CacheEntry{
					FirstLine: "HTTP/1.1 200 OK",
					Status:    200,
					Headers:   map[string]string{"Content-Type": "text/plain"},
					Data:      []byte(rec.body),
				})
				GCacheContainer.ReadCache(rec.key)
			}
		}(g)
	}
	writersDone.Wait()
	close(stop)
	background.Wait()

	// 落盘是异步的：等到每个键都进索引，而不是用 sleep 赌时间。
	keys := make([]string, 0, len(records))
	for _, rec := range records {
		keys = append(keys, rec.key)
	}
	waitAllIndexed(t, keys)

	for _, rec := range records {
		entry, ok := GCacheContainer.ReadCache(rec.key)
		if !ok {
			t.Fatalf("entry %s missing after concurrent writes", rec.key)
		}
		if string(entry.Data) != rec.body {
			t.Fatalf("entry %s body = %q, want %q", rec.key, entry.Data, rec.body)
		}
		node, ok := GCacheContainer.Index.Touch(rec.key)
		if !ok {
			t.Fatalf("entry %s dropped from index", rec.key)
		}
		if _, err := os.Stat(filepath.Join(node.Path, rec.key)); err != nil {
			t.Fatalf("entry %s not on disk: %v", rec.key, err)
		}
	}

	// 每个键只记一次容量：并发 admit 若重复累加，这里会大于真实字节数。
	GCacheContainer.mu.Lock()
	accounted := 0
	for _, rec := range records {
		accounted += GCacheContainer.sizes[rec.key]
	}
	total := GCacheContainer.totalSize
	GCacheContainer.mu.Unlock()

	want := 0
	for _, rec := range records {
		want += len(rec.body)
	}
	if accounted != want {
		t.Fatalf("accounted size = %d, want %d", accounted, want)
	}
	if total < accounted {
		t.Fatalf("totalSize = %d smaller than accounted %d", total, accounted)
	}
}

// 同一个键被多个协程反复覆盖写：索引里只能有一条，容量只算一份，
// 最终可回放的内容必须是某次写入的完整值（不能是交错的半截数据）。
func TestConcurrentWritesToSameKeyStayConsistent(t *testing.T) {
	const (
		writers = 8
		perG    = 10
	)
	key := GetMd5(t.Name())
	dir := t.TempDir()
	defer func() {
		GCacheContainer.Index.Remove(key)
		GCacheContainer.release(key)
		GCacheContainer.hotRemove(key)
	}()

	var wg sync.WaitGroup
	for g := 0; g < writers; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				// 等长正文：这样无论哪一次写入最后落盘，字节数记账都应相同。
				body := fmt.Sprintf("value-%03d", g*perG+i)
				GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{
					FirstLine: "HTTP/1.1 200 OK",
					Status:    200,
					Data:      []byte(body),
				})
				GCacheContainer.ReadCache(key)
			}
		}(g)
	}
	wg.Wait()
	// 80 次写入还在队列里陆续落盘；等队列排空再校验，
	// 否则 t.TempDir 的清理会和落盘协程抢文件句柄（Windows 上直接失败）。
	waitQueueDrained(t, dir)

	// 并发覆盖期间，读路径可能在文件正被截断的窗口里失败并把索引摘掉——
	// 这是允许的 miss 语义，不是必须命中。收尾串行写一次确定值，验证
	// "最后一次写入之后立即可读、正文完整、只记一份容量"。
	const finalBody = "final-value"
	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Data:      []byte(finalBody),
	})
	waitQueueDrained(t, dir)

	entry, ok := GCacheContainer.ReadCache(key)
	if !ok {
		t.Fatal("final write should be readable")
	}
	if string(entry.Data) != finalBody {
		t.Fatalf("body %q, want %q", entry.Data, finalBody)
	}
	if entry.Status != 200 || entry.FirstLine != "HTTP/1.1 200 OK" {
		t.Fatalf("entry metadata = %q/%d", entry.FirstLine, entry.Status)
	}

	GCacheContainer.mu.Lock()
	size, tracked := GCacheContainer.sizes[key]
	total := GCacheContainer.totalSize
	GCacheContainer.mu.Unlock()
	if !tracked {
		t.Fatal("key missing from size accounting")
	}
	if size != len(entry.Data) {
		t.Fatalf("accounted size = %d, want %d", size, len(entry.Data))
	}
	if total < size {
		t.Fatalf("totalSize = %d smaller than key size %d", total, size)
	}

	GCacheContainer.hotMu.Lock()
	_, hotOk := GCacheContainer.hot[key]
	hotEntries := GCacheContainer.hotEntries
	GCacheContainer.hotMu.Unlock()
	if !hotOk {
		t.Fatal("last written entry should be in the hot layer")
	}
	if hotEntries < 1 {
		t.Fatalf("hotEntries = %d", hotEntries)
	}
}

// waitQueueDrained 用一个哨兵键排空异步队列：CacheChan 是 FIFO，
// 哨兵进索引时，之前入队的条目都已经写完磁盘。
//
// 这里直接看索引而不是 ReadCache：读路径在"文件正被覆盖"的窗口里会按
// miss 语义把索引摘掉（见 ReadCache 的 unindex），拿它当队列进度会误判。
func waitQueueDrained(t *testing.T, dir string) {
	t.Helper()
	key := GetMd5(t.Name() + "/sentinel")
	defer func() {
		GCacheContainer.Index.Remove(key)
		GCacheContainer.release(key)
		GCacheContainer.hotRemove(key)
	}()
	GCacheContainer.WriteCache(key, 60, 0, dir, CacheEntry{
		FirstLine: "HTTP/1.1 200 OK",
		Status:    200,
		Data:      []byte("sentinel"),
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, ok := GCacheContainer.Index.Touch(key); ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("drain sentinel never indexed: queue=%d index=%d", len(CacheChan), GCacheContainer.Index.Len())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitAllIndexed 轮询等待异步落盘把每个键都写进索引。
func waitAllIndexed(t *testing.T, keys []string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		missing := 0
		for _, key := range keys {
			if _, ok := GCacheContainer.Index.Touch(key); !ok {
				missing++
			}
		}
		if missing == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d cache entries were never indexed (queue dropped them?)", missing, len(keys))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
