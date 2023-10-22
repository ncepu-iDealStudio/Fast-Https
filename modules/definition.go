/**
* @Author:刘浩宇
* @Description：各类模块接口定义
* @File：definition
* @Version:1.0.0
* @Date:2023/10/22 15:04:17
 */

package modules

type Logger interface {
	Info(msg string)
	Error(msg string)
	Debug(msg string)
	Warning(msg string)
}
