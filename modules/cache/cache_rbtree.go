package cache

import (
	"fmt"
	"sync"
	"time"

	rbt "github.com/emirpasic/gods/trees/redblacktree"
)

type redblackNode struct {
	cacheNode *CacheNode
}

type RBtree struct {
	tree *rbt.Tree
	mu   sync.RWMutex // for delete node
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

	t.mu.Lock()

	t.tree.Put(data.Md5, node)
	fmt.Println("this33333")

	t.mu.Unlock()
}

func (t *RBtree) RemoveFromRBtreeByKey(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tree.Remove(key)
}

func (t *RBtree) GetNodeFromRBtreeByKey(key string) (*CacheNode, bool) {
	t.mu.RLock()
	node_i, found := t.tree.Get(key)
	if !found {
		return nil, false
	}
	node := node_i.(*redblackNode)
	t.mu.RUnlock()
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
	t.mu.Lock()
	rbtree.cacheNode.Expire = int(time.Now().Add(time.Duration(expire)).Unix())
	t.mu.Unlock()
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
	t.mu.Lock()
	rbtree.cacheNode.Expire = int(time.Now().Add(time.Duration(expire)).Unix())
	rbtree.cacheNode.Path = path
	t.mu.Unlock()
	return true
}
