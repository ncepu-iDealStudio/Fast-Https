package cache

import "time"

// we will try having this func run in a go routine in future
// But we have to consider data consistency maybe with locks signals ...
func (CC *CacheContainer) ExpireCache() {
	curr_time := int(time.Now().Unix())

	// 遍历所有node，判断其是否过期，从而确定是否需要删除
	for _, key := range CC.RbRoot.Keys() {
		data := GetNodeFromRBtreeByKey(CC.RbRoot, key)
		if data == nil {
			continue
		}
		if curr_time > int(data.Expire) {
			RemoveFromDisk(*data)
			RemoveFromRBtreeByKey(CC.RbRoot, key)
		}
	}

	// min_node := FindFirst(CC.RbRoot.node)
	// if curr_time < int(min_node.key) {
	// 	return
	// } else {
	// 	// remove file in desk
	// 	RemoveFromDisk(*min_node.RbCacheNode)
	// 	// remove node in rbtree
	// 	DeleteInRbtree(CC.RbRoot, min_node)
	// }
	// to do: implent a function that find next to expire
}
