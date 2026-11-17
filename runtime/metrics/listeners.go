package metrics

import "sync/atomic"

// ListenerInfo 是监听端口的只读副本。只含端口、ssl/h2 与在途请求数，
// 不含证书路径。
type ListenerInfo struct {
	Port string `json:"port"`
	SSL  bool   `json:"ssl,omitempty"`
	H2   bool   `json:"h2,omitempty"`
	// InFlightRequests 是该端口正在处理的 HTTP 请求数。端口表副本仍由
	// reload 整体替换；这个字段在 TakeSnapshot 时按注册的实时来源填充。
	InFlightRequests int64 `json:"in_flight_requests"`
}

var listeners atomic.Pointer[[]ListenerInfo]

// SetListenerInFlightSource 注册在途请求数的实时来源。组合根在监听建立后
// 注册一个读取 listener 表的函数，让 /runtime 快照拿到实时值，而 metrics
// 不反向依赖 runtime/listener。传 nil 取消注册。
func SetListenerInFlightSource(fn func() map[string]int64) {
	if fn == nil {
		inFlightSource.Store(nil)
		return
	}
	inFlightSource.Store(&fn)
}

var inFlightSource atomic.Pointer[func() map[string]int64]

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

// listenersWithInFlight 返回端口表副本，并按实时来源填好各端口的在途请求数。
func listenersWithInFlight() []ListenerInfo {
	out := Listeners()
	fn := inFlightSource.Load()
	if fn == nil || len(out) == 0 {
		return out
	}
	live := (*fn)()
	for i := range out {
		out[i].InFlightRequests = live[out[i].Port]
	}
	return out
}
