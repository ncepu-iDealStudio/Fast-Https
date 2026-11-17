package model

import "sync/atomic"

// 运行时配置快照。reload 校验通过后整体替换快照，读取端要么看到完整的
// 旧配置、要么看到完整的新配置，失败时旧快照保持不动。
// （架构收敛计划阶段 4b：消除对进程级全局配置的无同步读写。）
var (
	currentConfig atomic.Pointer[Fast_Https]
	currentMIME   atomic.Pointer[map[string]string]
)

func init() {
	currentConfig.Store(&Fast_Https{})
	emptyMIME := map[string]string{}
	currentMIME.Store(&emptyMIME)
}

// Current 返回当前生效的配置快照。尚未加载配置时是零值快照。
func Current() *Fast_Https {
	return currentConfig.Load()
}

// MIME 返回当前生效的 MIME 表。表在发布后不再修改，读取方无需加锁。
func MIME() map[string]string {
	return *currentMIME.Load()
}

// Store 替换配置快照。加载入口在校验全部通过后调用；
// 导出是为了测试能够保存与恢复快照。
func Store(cfg *Fast_Https, mime map[string]string) {
	if cfg != nil {
		currentConfig.Store(cfg)
	}
	if mime != nil {
		currentMIME.Store(&mime)
	}
}
