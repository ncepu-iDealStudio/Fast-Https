package metrics

import "sync/atomic"

// ListenerInfo 是监听端口的只读副本。只含端口和 ssl/h2，不含证书路径。
type ListenerInfo struct {
	Port string `json:"port"`
	SSL  bool   `json:"ssl,omitempty"`
	H2   bool   `json:"h2,omitempty"`
}

var listeners atomic.Pointer[[]ListenerInfo]

// ReplaceListeners 用一份拷贝整体替换端口表。reload 时调用，避免快照去读
// 无锁赋值的 Server.Listens。
func ReplaceListeners(list []ListenerInfo) {
	cp := make([]ListenerInfo, len(list))
	copy(cp, list)
	listeners.Store(&cp)
}

// Listeners 返回当前端口表的拷贝。尚未发布过则返回 nil。
func Listeners() []ListenerInfo {
	p := listeners.Load()
	if p == nil {
		return nil
	}
	out := make([]ListenerInfo, len(*p))
	copy(out, *p)
	return out
}
