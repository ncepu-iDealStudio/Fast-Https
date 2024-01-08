/**
* @Author:刘浩宇
* @Description：用于测试模块是否可以正确注册，获取以及执行
* @File：log_test
* @Version:1.0.0
* @Date:2023/10/22 16:18:28
 */

package modules

import (
	"context"
	"fast-https/modules"
	_ "fast-https/modules/logging" // 模块导入
	"fast-https/modules/workchain"
	_ "fast-https/modules/workchain/example"
	"testing"
)

func TestLogPlugin(t *testing.T) {
	// 模块名称可通过配置文件获取
	moduleName := "fast.plugin.DefaultLogger"
	// 测试模块的获取
	module, err := modules.GetModule[modules.Logger](moduleName)
	if err != nil {
		t.Errorf("Module Type assertion is wrong")
	}
	t.Log("get logger module")
	// 测试模块的使用
	module.Warning("this is one test example")
}

func TestWorkChain(t *testing.T) {
	c := workchain.NewContext(context.Background())
	m1, _ := modules.GetModule[workchain.Handler]("fast.process.Gizmo")
	m2, _ := modules.GetModule[workchain.Handler]("fast.process.error")
	m3, _ := modules.GetModule[workchain.Handler]("fast.process.cache")
	c.Use(m3)
	c.Use(m2, m1) // 定义了一条工作链，工作顺序从左往右，从上往下
	c.Next()      // 开始执行工作链
}
