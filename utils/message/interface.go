package message

import (
	"fmt"
)

type message struct {
	Context any
	Type    string
}

// send 在持有读锁的前提下把消息投递到输出通道；通道未初始化时直接丢弃。
func send(msg message) {
	if !rwMutex.TryRLock() {
		return
	}
	defer rwMutex.RUnlock()
	if outputChan == nil {
		return
	}
	outputChan.In <- msg
}

// PrintInfo 输出普通信息日志。
func PrintInfo(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "info",
	})
}

// PrintWarn 输出告警日志。
func PrintWarn(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "warn",
	})
}

// PrintErr 输出错误日志。
func PrintErr(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "err",
	})
}

// PrintRecover 输出 panic 恢复时捕获到的信息。
func PrintRecover(a any) {
	send(message{
		Context: a,
		Type:    "recover",
	})
}

// PrintAccess 输出访问日志：host 为访问地址，a 为其余日志内容。
func PrintAccess(host string, a ...interface{}) {
	context := map[string]any{
		"host":    host,
		"message": a,
	}
	send(message{
		Context: context,
		Type:    "access",
	})
}
