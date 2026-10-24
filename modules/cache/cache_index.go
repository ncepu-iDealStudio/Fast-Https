package cache

import (
	"sync"
	"time"
)

// CacheIndex 是缓存条目的内存索引：缓存键 -> 元数据。
//
// 索引只需要按键增删查和全量过期扫描，map 的 O(1) 增删查完全够用，
// 有序结构反而每次写都要维护树形。过期清理协程每秒扫描一次，
// 与请求协程的读写并发，因此全部访问都持锁。
type CacheIndex struct {
	mu    sync.RWMutex
	items map[string]*CacheNode
}

// NewCacheIndex 创建一个空的缓存索引。
func NewCacheIndex() *CacheIndex {
	return &CacheIndex{items: make(map[string]*CacheNode)}
}

// Set 写入或覆盖一个索引节点。
func (idx *CacheIndex) Set(data *CacheNode) {
	idx.mu.Lock()
	idx.items[data.Md5] = data
	idx.mu.Unlock()
}

// Remove 按键摘除索引节点。
func (idx *CacheIndex) Remove(key string) {
	idx.mu.Lock()
	delete(idx.items, key)
	idx.mu.Unlock()
}

// Touch 取出节点并把过期时间顺延一个有效期，实现"读即续期"。
// 这里必须持写锁：续期修改的是节点字段，用读锁会造成数据竞争。
func (idx *CacheIndex) Touch(key string) (*CacheNode, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	node, found := idx.items[key]
	if !found {
		return nil, false
	}
	node.Expire = int(time.Now().Unix()) + node.Valid
	return node, true
}

// TakeExpired 取出并摘除全部已过期节点，供清理协程删除磁盘文件。
func (idx *CacheIndex) TakeExpired(now int) []*CacheNode {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	var expired []*CacheNode
	for key, node := range idx.items {
		if now > node.Expire {
			expired = append(expired, node)
			delete(idx.items, key)
		}
	}
	return expired
}

// Len 返回当前索引条目数。
func (idx *CacheIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.items)
}
