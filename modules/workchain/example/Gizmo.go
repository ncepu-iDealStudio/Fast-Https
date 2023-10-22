/**
* @Author:刘浩宇
* @Description：
* @File：Gizmo.go
* @Version:1.0.0
* @Date:2023/10/22 17:00:04
 */

package example

import (
	"fast-https/modules"
	"fast-https/modules/workchain"
	"fmt"
)

type Gizmo struct {
	Title string
}

func init() {
	modules.RegisterModule(&Gizmo{})
}

func (g *Gizmo) FastModule() *modules.ModuleInfo {
	return &modules.ModuleInfo{
		ID: "fast.process.Gizmo",
		New: func() modules.Module {
			return &Gizmo{}
		},
	}
}

func (g *Gizmo) Handle(ctx *workchain.WorkChain) {
	g.Title = "Hello World"
	fmt.Printf("This is Gizmo process Gizmo is = %v\n", g)
	ctx.Next()
}
