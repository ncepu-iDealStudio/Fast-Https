/**
* @Author:刘浩宇
* @Description：用于测试模块是否可以正确注册，获取以及执行
* @File：log_test
* @Version:1.0.0
* @Date:2023/10/22 16:18:28
 */

package modules

import (
	"fast-https/modules"
	_ "fast-https/modules/logging" // 模块导入
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
