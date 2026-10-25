// Package cache 实现反向代理响应缓存：以请求 MD5 为键落盘，内存用 map 索引过期时间。
//
// 一条缓存由两部分组成：内存里的 CacheNode 只记录过期时间与落盘位置，
// 磁盘上的 CacheEntry 保存可直接回放的响应（状态行、头部、响应体）。
package cache

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"fast-https/config"
	"fast-https/infra/logging"
	"os"
	"path/filepath"
	"container/list"
	"sync"
	"time"
)

// cacheQueueSize 是异步落盘队列长度。队列满时丢弃本次写入，而不是阻塞请求协程。
const cacheQueueSize = 1000

// CacheNode 是内存索引里的一条缓存元数据。
type CacheNode struct {
	Expire int    // 过期时刻，Unix 秒
	Valid  int    // 有效时长，秒；命中后按它顺延过期时间
	Path   string // 落盘目录
	Md5    string // 缓存键
}

// CacheHead 是 CacheNode 在磁盘条目里的副本。
type CacheHead CacheNode

// CacheEntry 是一条缓存的完整内容，可直接还原成响应。
type CacheEntry struct {
	Head      CacheHead
	FirstLine string            // 例如 HTTP/1.1 200 OK
	Status    int               // 状态码，用于访问日志
	Headers   map[string]string // 已剔除逐跳头
	Data      []byte
	Size      int
}

// 热条目内存副本的容量上限：条数与字节双限制，超限按 LRU 淘汰。
// 磁盘条目保持完整，热层只是命中的加速层。
const (
	hotMaxEntries = 4096
	hotMaxBytes   = 64 << 20
)

// CacheContainer 持有缓存索引、容量记账与热条目副本。
type CacheContainer struct {
	Index *CacheIndex

	mu        sync.Mutex
	sizes     map[string]int
	totalSize int
	warned    bool

	// hot 是磁盘条目的内存副本：命中不再开文件 + gob 解码。
	// hotOrder 前端是最近使用的，超限从尾部淘汰。
	hotMu      sync.Mutex
	hot        map[string]*hotEntry
	hotOrder   *list.List
	hotEntries int
	hotBytes   int
	hotCap     int
	hotCapByte int
}

// hotEntry 是热层里的一条缓存：磁盘 CacheEntry 的共享只读副本。
type hotEntry struct {
	key     string
	entry   *CacheEntry
	element *list.Element
}

// hotSize 记一条热条目占用的字节数。
func hotSize(he *hotEntry) int {
	return len(he.entry.Data) + len(he.key) + 128
}

var (
	// CacheChan 是异步落盘队列。
	CacheChan chan CacheEntry
	// GCacheContainer 是进程内唯一的缓存容器。
	GCacheContainer *CacheContainer
)

func init() {
	CacheChan = make(chan CacheEntry, cacheQueueSize)
	GCacheContainer = NewCacheContainer()
	go GCacheContainer.WriteToDisk()
}

// NewCacheContainer 创建一个空的缓存容器。
func NewCacheContainer() *CacheContainer {
	return newCacheContainerWithCaps(hotMaxEntries, hotMaxBytes)
}

func newCacheContainerWithCaps(hotEntries, hotBytes int) *CacheContainer {
	return &CacheContainer{
		Index:      NewCacheIndex(),
		sizes:      make(map[string]int),
		hot:        make(map[string]*hotEntry),
		hotOrder:   list.New(),
		hotCap:     hotEntries,
		hotCapByte: hotBytes,
	}
}

// GetMd5 计算缓存键。缓存键由 proxy_cache.key 规则串生成。
func GetMd5(str string) string {
	sum := md5.Sum([]byte(str))
	return hex.EncodeToString(sum[:])
}

// WriteToDisk 持续消费落盘队列。
//
// 索引在文件写成功之后才建立：否则未完成的写入会被读路径当成损坏条目。
// 写成功的条目同时进入热层，首个请求即命中内存。
func (CC *CacheContainer) WriteToDisk() {
	for entry := range CacheChan {
		if err := writeEntryToDisk(entry); err != nil {
			logging.PrintErr("proxy cache write failed: ", err)
			CC.release(entry.Head.Md5)
			continue
		}
		node := CacheNode(entry.Head)
		CC.Index.Set(&node)
		CC.hotPut(node.Md5, &entry)
	}
}

// writeEntryToDisk 把一条缓存写进 entry.Head.Path 目录。
func writeEntryToDisk(entry CacheEntry) error {
	savePath := entry.Head.Path
	if err := os.MkdirAll(savePath, 0o755); err != nil {
		return err
	}
	realPath := filepath.Join(savePath, entry.Head.Md5)
	// O_TRUNC 必须带上：覆盖成更短的内容时，否则旧尾部会留在文件里。
	file, err := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	defer file.Close()
	return gob.NewEncoder(file).Encode(entry)
}

// readEntry 从磁盘读取一条完整缓存。
func readEntry(realPath string) (*CacheEntry, error) {
	file, err := os.Open(realPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var entry CacheEntry
	if err := gob.NewDecoder(file).Decode(&entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// readEntryHead 只读取索引头，避免启动加载时把响应体全部分配出来。
func readEntryHead(realPath string) (CacheHead, string, int, error) {
	file, err := os.Open(realPath)
	if err != nil {
		return CacheHead{}, "", 0, err
	}
	defer file.Close()
	var head struct {
		Head      CacheHead
		FirstLine string
		Size      int
	}
	if err := gob.NewDecoder(file).Decode(&head); err != nil {
		return CacheHead{}, "", 0, err
	}
	return head.Head, head.FirstLine, head.Size, nil
}

// LoadCache 在启动时扫描配置里的缓存目录，重建内存索引。
func (CC *CacheContainer) LoadCache() {
	for _, server := range config.Current().Servers {
		for _, path := range server.Path {
			if path.ProxyCache.Path == "" {
				continue
			}
			files, err := GetDirFiles(path.ProxyCache.Path)
			if err != nil {
				logging.PrintWarn("proxy cache scan failed: ", err)
				continue
			}
			for _, file := range files {
				head, firstLine, size, err := readEntryHead(file)
				if err != nil {
					logging.PrintWarn("proxy cache skip unreadable file: ", err)
					continue
				}
				// 旧格式条目没有可回放的响应行，丢弃而不进索引。
				if firstLine == "" || head.Md5 == "" {
					continue
				}
				node := CacheNode(head)
				CC.Index.Set(&node)
				CC.admit(node.Md5, size, 0)
			}
		}
	}
}

// WriteCache 写入一条缓存。
//
// key 为请求 MD5，expire 为有效秒数，maxSize 为 0 表示不限制容量。
// 容量上限是软上限：超限时丢弃本次写入，空间由过期清理回收，不做淘汰扫描。
func (CC *CacheContainer) WriteCache(key string, expire, maxSize int, path string, entry CacheEntry) {
	if key == "" || expire <= 0 || path == "" {
		return
	}
	if len(key) < 3 {
		return
	}

	if !CC.admit(key, len(entry.Data), maxSize) {
		return
	}

	now := int(time.Now().Unix())
	node := CacheNode{
		Expire: now + expire,
		Valid:  expire,
		Md5:    key,
		// 按 md5 尾部两位做目录分片，避免单目录文件过多。
		Path: filepath.Join(path, key[len(key)-1:], key[len(key)-3:len(key)-1]),
	}
	entry.Head = CacheHead(node)
	entry.Size = len(entry.Data)

	select {
	case CacheChan <- entry:
	default:
		// 队列满时回滚容量记账；索引本来还没建立，无需回滚。
		CC.release(key)
		logging.PrintWarn("proxy cache queue full, dropped entry ", key)
	}
}

// ReadCache 读取一条缓存，命中时顺延过期时间（读即续期）。
// 热层命中直接返回内存副本，未命中才读盘；索引与磁盘不一致或
// 遇到旧格式条目时按未命中处理，并清理索引与热层。
func (CC *CacheContainer) ReadCache(key string) (*CacheEntry, bool) {
	node, ok := CC.Index.Touch(key)
	if !ok {
		CC.hotRemove(key)
		return nil, false
	}

	if entry := CC.hotGet(key); entry != nil {
		return entry, true
	}

	entry, err := readEntry(filepath.Join(node.Path, key))
	if err != nil {
		logging.PrintWarn("proxy cache read failed: ", err)
		CC.unindex(key)
		return nil, false
	}
	if entry.FirstLine == "" {
		// 旧格式条目：没有状态行与头部，无法回放。
		CC.unindex(key)
		return nil, false
	}
	CC.hotPut(key, entry)
	return entry, true
}

// ExpireCache 清理已过期条目：先在索引里摘除，再删除磁盘文件并释放
// 容量记账与热层副本。
func (CC *CacheContainer) ExpireCache() {
	now := int(time.Now().Unix())
	for _, node := range CC.Index.TakeExpired(now) {
		RemoveFromDisk(*node)
		CC.release(node.Md5)
		CC.hotRemove(node.Md5)
	}
}

// RemoveFromDisk 删除一条缓存的磁盘文件。
func RemoveFromDisk(node CacheNode) {
	_ = os.Remove(filepath.Join(node.Path, node.Md5))
}

// unindex 只摘除索引、容量记账与热层副本，不动磁盘文件。
// 文件可能正在被覆盖写入，删文件会与写协程竞争。
func (CC *CacheContainer) unindex(key string) {
	CC.Index.Remove(key)
	CC.release(key)
	CC.hotRemove(key)
}

// ---------- 热条目内存副本（LRU） ----------

// hotGet 返回热副本并把条目移到最近使用端。
func (CC *CacheContainer) hotGet(key string) *CacheEntry {
	CC.hotMu.Lock()
	defer CC.hotMu.Unlock()
	he, ok := CC.hot[key]
	if !ok {
		return nil
	}
	CC.hotOrder.MoveToFront(he.element)
	return he.entry
}

// hotPut 放入或刷新热副本，超限从最近最少使用端淘汰。
func (CC *CacheContainer) hotPut(key string, entry *CacheEntry) {
	CC.hotMu.Lock()
	defer CC.hotMu.Unlock()

	if he, ok := CC.hot[key]; ok {
		CC.hotBytes -= hotSize(he)
		he.entry = entry
		CC.hotBytes += hotSize(he)
		CC.hotOrder.MoveToFront(he.element)
		return
	}

	he := &hotEntry{key: key, entry: entry}
	he.element = CC.hotOrder.PushFront(he)
	CC.hot[key] = he
	CC.hotEntries++
	CC.hotBytes += hotSize(he)

	for (CC.hotEntries > CC.hotCap || CC.hotBytes > CC.hotCapByte) && CC.hotOrder.Len() > 0 {
		oldest := CC.hotOrder.Back()
		if oldest == nil {
			break
		}
		victim := oldest.Value.(*hotEntry)
		CC.hotOrder.Remove(oldest)
		delete(CC.hot, victim.key)
		CC.hotEntries--
		CC.hotBytes -= hotSize(victim)
	}
}

// hotRemove 从热层摘除一个键。
func (CC *CacheContainer) hotRemove(key string) {
	CC.hotMu.Lock()
	defer CC.hotMu.Unlock()
	he, ok := CC.hot[key]
	if !ok {
		return
	}
	CC.hotOrder.Remove(he.element)
	delete(CC.hot, he.key)
	CC.hotEntries--
	CC.hotBytes -= hotSize(he)
}

// admit 做容量记账，返回是否允许写入。
func (CC *CacheContainer) admit(key string, size, maxSize int) bool {
	CC.mu.Lock()
	defer CC.mu.Unlock()

	previous := CC.sizes[key]
	if maxSize > 0 && CC.totalSize-previous+size > maxSize {
		if !CC.warned {
			CC.warned = true
			logging.PrintWarn("proxy cache reached max_size, new entries are not cached until space is reclaimed")
		}
		return false
	}
	CC.warned = false
	CC.sizes[key] = size
	CC.totalSize += size - previous
	return true
}

// release 归还某个键占用的容量。
func (CC *CacheContainer) release(key string) {
	CC.mu.Lock()
	defer CC.mu.Unlock()
	if size, ok := CC.sizes[key]; ok {
		CC.totalSize -= size
		delete(CC.sizes, key)
	}
}

// GetDirFiles 列出目录下所有文件。
func GetDirFiles(path string) ([]string, error) {
	var names []string
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			names = append(names, filePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}
