package core

import (
	"fast-https/runtime/listener"
	"maps"
	"slices"
	"sync"
)

// registry 保存 location 类型码到处理函数组的映射。
//
// 这里用 map 而不是定长数组：类型码由配置层定义，数量不应被数据结构限制，
// 新增一种 location 类型只需在组合根注册，不必同步修改数组长度。
var (
	registryMu sync.RWMutex
	registry   = map[int]RRcircleHandler{}
)

// RegisterHandler 注册某种 location 类型的处理函数组。
// filter 或 handler 传 nil 表示该类型不参与请求分发；cmd 传 nil 使用默认命令解析器。
// 重复注册会覆盖旧值，因此 reload 后可以安全重建。
func RegisterHandler(pathType int, filter func(*listener.ListenCfg, *Event) bool,
	handler func(*listener.ListenCfg, *Event), cmd func(*listener.ListenCfg, *Event)) {
	registryMu.Lock()
	defer registryMu.Unlock()

	// nil 视为"撤销注册"，避免残留旧处理函数继续处理新配置。
	if filter == nil && handler == nil {
		delete(registry, pathType)
		return
	}
	if cmd == nil {
		cmd = DefaultParseCommandHandler
	}
	registry[pathType] = RRcircleHandler{
		ParseCommandHandler: cmd,
		FilterHandler:       filter,
		RRHandler:           handler,
	}
}

// Handler 返回某种 location 类型注册的处理函数组。
func Handler(pathType int) (RRcircleHandler, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	h, ok := registry[pathType]
	return h, ok
}

// RegisteredTypes 返回已注册的类型码，升序，便于日志与诊断。
func RegisteredTypes() []int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return slices.Sorted(maps.Keys(registry))
}
