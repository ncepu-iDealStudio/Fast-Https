/**
* @Author: yizhigopher
* @Description: 对红黑树每个结点进行判断，删去过期结点
* @File: cache_validate.go
* @Version: 1.0.0
* @Date: 2023/10/19 21:38:41
 */

package cache

import (
	"time"
)

// import "time"

// we will try having this func run in a go routine in future
// But we have to consider data consistency maybe with locks signals ...
func (CC *CacheContainer) ExpireCache() {
	curr_time := int(time.Now().Unix())

	// 遍历所有node，判断其是否过期，从而确定是否需要删除
	for _, rbnode := range CC.RbRoot.tree.Values() {
		data := rbnode.(*redblackNode).cacheNode
		if curr_time > int(data.Expire) {
			// fmt.Println("-----" + data.Md5 + " is expired...")
			RemoveFromDisk(*data)
			CC.RbRoot.RemoveFromRBtreeByKey(data.Md5)
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
