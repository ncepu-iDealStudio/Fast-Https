package cache

import "time"

// we will try having this func run in a go routine in future
// But we have to consider data consistency maybe with locks signals ...
func (CC *CacheContainer) ExpireCache() {
	curr_time := int(time.Now().Unix())
	min_node := FindFirst(CC.RbRoot.node)
	if curr_time < int(min_node.key) {
		return
	} else {
		// remove file in desk
		RemoveFromDisk(*min_node.RbCacheNode)
		// remove node in rbtree
		DeleteInRbtree(CC.RbRoot, min_node)
	}
	// to do: implent a function that find next to expire
}
