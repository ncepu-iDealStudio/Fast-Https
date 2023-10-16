package cache

import (
	"sync"
	"time"

	rbt "github.com/emirpasic/gods/trees/redblacktree"
)

type redblackNode struct {
	mu        sync.RWMutex // 为每个结点增加一个读写锁
	cacheNode *CacheNode
}

type RBtree struct {
	tree *rbt.Tree
	mu   sync.Mutex // for delete node
}

func NewRBtree() *RBtree {
	return &RBtree{
		tree: rbt.NewWithStringComparator(),
	}
}

func (t *RBtree) AddInRbtree(data *CacheNode) {
	node := &redblackNode{
		cacheNode: data,
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	t.tree.Put(data.Md5, node)
}

func (t *RBtree) RemoveFromRBtreeByKey(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tree.Remove(key)
}

func (t *RBtree) GetNodeFromRBtreeByKey(key string) (*CacheNode, bool) {
	node_i, found := t.tree.Get(key)
	if !found {
		return nil, false
	}
	node := node_i.(*redblackNode)
	node.mu.RLock()
	defer node.mu.RUnlock()
	return node.cacheNode, true
}

func (t *RBtree) IsExpired(key string) (*CacheNode, bool) {
	res, found := t.GetNodeFromRBtreeByKey(key)
	if !found {
		return nil, false
	}
	if int(time.Now().Unix()) > int(res.Expire) {
		return res, true
	}
	return nil, false
}

func (t *RBtree) UpdateExpire(key string, expire int) bool {
	value, found := t.tree.Get(key)
	if !found {
		return false
	}
	rbtree := value.(*redblackNode)
	rbtree.mu.Lock()
	rbtree.cacheNode.Expire = int(time.Now().Add(time.Duration(expire)).Unix())
	rbtree.mu.Unlock()
	return true
}

/*
This method also updates the expiration time,
so you need to pass the expire.
*/
func (t *RBtree) UpdatePath(key string, path string, expire int) bool {
	value, found := t.tree.Get(key)
	if !found {
		return false
	}
	rbtree := value.(*redblackNode)
	rbtree.mu.Lock()
	rbtree.cacheNode.Expire = int(time.Now().Add(time.Duration(expire)).Unix())
	rbtree.cacheNode.Path = path
	rbtree.mu.Unlock()
	return true
}
