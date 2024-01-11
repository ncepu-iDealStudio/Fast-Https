/**
* @Author: yizhigopher
* @Description: 红黑树实现
* @File: cache_rbtree.go
* @Version: 1.0.0
* @Date: 2023/10/19 21:37:48
 */

package cache

import (
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
	t.mu.Unlock()
}

func (t *RBtree) RemoveFromRBtreeByKey(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tree.Remove(key)
}

func (t *RBtree) GetNodeFromRBtreeByKey(key string) (*CacheNode, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	node_i, found := t.tree.Get(key)
	if !found {
		return nil, false
	}
	node := node_i.(*redblackNode)
	node.cacheNode.Expire = int(time.Now().Unix()) + node.cacheNode.Valid
	return node.cacheNode, true
}

func (t *RBtree) UpdateExpire(key string, expire int) bool {
	t.mu.Lock()
	value, found := t.tree.Get(key)
	if !found {
		t.mu.Unlock()
		return false
	}
	rbtree := value.(*redblackNode)
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
