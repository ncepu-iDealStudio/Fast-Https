/**
* @Author:刘浩宇
* @Description：这个仅是示例
* @File：Cache.go
* @Version:1.0.0
* @Date:2023/10/22 17:02:00
 */

package example

import (
	"fast-https/modules"
	"fast-https/modules/workchain"
	"fmt"
)

type Cache struct {
	md5  string
	path string
}

func init() {
	modules.RegisterModule(&Cache{})
}

func (c *Cache) FastModule() *modules.ModuleInfo {
	return &modules.ModuleInfo{
		ID: "fast.process.cache",
		New: func() modules.Module {
			return &Cache{}
		},
	}
}

func (c *Cache) Handle(ctx *workchain.WorkChain) {
	c.md5 = "12123"
	c.path = "/fav.ico"
	fmt.Printf("write cache %v into red-black-tree\n", c)
	ctx.Next()
}
