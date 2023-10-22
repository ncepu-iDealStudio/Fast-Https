/**
* @Author:刘浩宇
* @Description：模块部分定义
* @File：module
* @Version:1.0.0
* @Date:2023/10/22 14:59:32
 */

package modules

import (
	"fmt"
	"os"
)

type Module interface {
	FastModule() *ModuleInfo
}

type ModuleInfo struct {
	ID  string
	New func() Module // 模块初始化
}

var moduleRepository = make(map[string]*ModuleInfo)

// RegisterModule 用来将模块注册到map表中，便于之后的获取操作
func RegisterModule(instance Module) bool {
	mod := instance.FastModule()
	_, ok := moduleRepository[mod.ID]
	if ok {
		fmt.Printf("module %s has been registered\n", mod.ID)
		return false
	}
	moduleRepository[mod.ID] = mod
	return true
}

// validate 用来判断模块是否已经注册了。
// 如果没有注册，程序中止，并在控制台打印未注册模块的名称
func validate(id string) bool {
	_, ok := moduleRepository[id]
	if !ok {
		fmt.Printf("module %s is not registered, please register it", id)
		os.Exit(1)
	}
	return ok
}

// GetModule 通过模块名称以及指定的类型获取相应的模块
// 会先执行validate函数，确保不会出现空地址异常
func GetModule[T any](id string) (T, error) {
	validate(id)
	var zero T
	m := moduleRepository[id]
	plugin := m.New()
	t, ok := plugin.(T)
	if !ok {
		return zero, fmt.Errorf("module type mismatch")
	}
	return t, nil
}
