/**
* @Author:刘浩宇
* @Description：用于展示出现错误后，中止工作链的行为
* @File：withError
* @Version:1.0.0
* @Date:2023/10/22 17:06:28
 */

package example

import (
	"fast-https/modules"
	"fast-https/modules/workchain"
	"fmt"
)

type ErrorW struct {
	err error
}

func init() {
	modules.RegisterModule(&ErrorW{})
}

func (e ErrorW) FastModule() *modules.ModuleInfo {
	return &modules.ModuleInfo{
		ID: "fast.process.error",
		New: func() modules.Module {
			return &ErrorW{err: fmt.Errorf("this is a error")}
		},
	}
}

func (e ErrorW) Handle(ctx *workchain.WorkChain) {
	fmt.Println(e.err.Error())
	if e.err != nil {
		ctx.Abort()
	}
	ctx.Next()
}
