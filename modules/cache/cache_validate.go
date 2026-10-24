package cache

import (
	"time"
)

// ExpireCache 清理已过期条目：先在索引里摘除，再删除磁盘文件。
//
// 索引已持有锁，调用方不必再加锁。
func (CC *CacheContainer) ExpireCache() {
	now := int(time.Now().Unix())
	for _, node := range CC.RbRoot.TakeExpired(now) {
		RemoveFromDisk(*node)
		CC.release(node.Md5)
	}
}
