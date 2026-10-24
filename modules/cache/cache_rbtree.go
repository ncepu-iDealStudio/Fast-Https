package cache

import (
	"sync"
	"time"

	rbt "github.com/emirpasic/gods/trees/redblacktree"
)

type redblackNode struct {
	cacheNode *CacheNode
}

// RBtree 是缓存条目索引。所有对底层树的读写都必须持锁：
// 清理协程每秒遍历一次，与请求协程的读写是并发的。
type RBtree struct {
	tree *rbt.Tree
	mu   sync.RWMutex
}

// NewRBtree 创建一个以缓存键为索引的红黑树。
func NewRBtree() *RBtree {
	return &RBtree{
		tree: rbt.NewWithStringComparator(),
	}
}

// AddInRbtree 写入或覆盖一个索引节点。
func (t *RBtree) AddInRbtree(data *CacheNode) {
	node := &redblackNode{cacheNode: data}
	t.mu.Lock()
	t.tree.Put(data.Md5, node)
	t.mu.Unlock()
}

// RemoveFromRBtreeByKey 按键摘除索引节点。
func (t *RBtree) RemoveFromRBtreeByKey(key string) {
	t.mu.Lock()
	t.tree.Remove(key)
	t.mu.Unlock()
}

// TouchNode 取出节点并把过期时间顺延一个有效期，实现"读即续期"。
//
// 这里必须持写锁：续期修改的是节点字段，用读锁会造成数据竞争。
func (t *RBtree) TouchNode(key string) (*CacheNode, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	value, found := t.tree.Get(key)
	if !found {
		return nil, false
	}
	node := value.(*redblackNode).cacheNode
	node.Expire = int(time.Now().Unix()) + node.Valid
	return node, true
}

// TakeExpired 取出并摘除全部已过期节点，供清理协程删除磁盘文件。
func (t *RBtree) TakeExpired(now int) []*CacheNode {
	t.mu.Lock()
	defer t.mu.Unlock()

	var expired []*CacheNode
	for _, value := range t.tree.Values() {
		node := value.(*redblackNode).cacheNode
		if now > node.Expire {
			expired = append(expired, node)
		}
	}
	for _, node := range expired {
		t.tree.Remove(node.Md5)
	}
	return expired
}

// Len 返回当前索引条目数。
func (t *RBtree) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tree.Size()
}
