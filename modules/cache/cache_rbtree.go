package cache

import (
	rbt "github.com/emirpasic/gods/trees/redblacktree"
)

func NewRBtree() *rbt.Tree {
	return rbt.NewWithStringComparator()
}

func AddInRbtree(tree *rbt.Tree, node *CacheNode) {
	tree.Put(node.Md5, node)
}

func RemoveFromRBtreeByKey(tree *rbt.Tree, key any) {
	tree.Remove(key)
}

func GetNodeFromRBtreeByKey(tree *rbt.Tree, key any) *CacheNode {
	node, ok := tree.Get(key)
	if !ok { // 没找到key对应的结点
		return nil
	}
	// 返回Cache结点信息
	return node.(*CacheNode)
}
