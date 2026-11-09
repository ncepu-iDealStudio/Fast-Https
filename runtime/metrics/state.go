// Package metrics 保存进程内可原子读取的运行状态：生命周期、连接计数、
// 请求与状态码、reload 结果和监听端口副本。读取只做原子 Load，不持 Server.mu，
// 也不去读无锁的 Server.Listens。
//
// 快照由 TakeSnapshot() 组装，HTTP 出口在组合根 cmd 的 127.0.0.1:9088/runtime。
package metrics

import (
	"os"
	"sync/atomic"
	"time"

	"fast-https/config"
)

// StateCode 是生命周期枚举。对外快照映射成字符串；不能用空串表示 starting，
// 因为字符串零值就是空串。整数零值对应 StateStarting，init 再显式写入一次。
type StateCode int32

const (
	StateStarting StateCode = iota
	StateRunning
	StateReloading
	StateStopping
)

func (s StateCode) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateReloading:
		return "reloading"
	case StateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

var (
	state                atomic.Int32
	startedAtUnixNano    atomic.Int64
	pid                  atomic.Int64
	version              atomic.Value // string
	configGeneration     atomic.Uint64
	lastReloadOK         atomic.Bool
	lastReloadAtUnixNano atomic.Int64
	reloadFailedTotal    atomic.Uint64
	stopTimeoutTotal     atomic.Uint64
)

func init() {
	state.Store(int32(StateStarting))
	version.Store("")
}

// State 返回当前生命周期。
func State() StateCode {
	return StateCode(state.Load())
}

// RecordProcessStart 在 ServerInit 记下启动时刻、PID 和编译期版本。
// reload 不得再次调用，否则 Monitor 会把热更新误判成重启。
func RecordProcessStart() {
	startedAtUnixNano.Store(time.Now().UnixNano())
	pid.Store(int64(os.Getpid()))
	version.Store(config.CURRENT_VERSION)
}

// MarkRunning 在监听建立后、以及 Reload 结束（成功或失败）时回到 running。
func MarkRunning() {
	state.Store(int32(StateRunning))
}

// MarkReloading 在 Reload 开头进入 reloading。
func MarkReloading() {
	state.Store(int32(StateReloading))
}

// MarkStopping 在优雅停机开始时进入 stopping，不再回到 running。
func MarkStopping() {
	state.Store(int32(StateStopping))
}

// NoteReloadSuccess 在配置与监听都切换完成后调用：生成号加一，记下成功与时间。
func NoteReloadSuccess() {
	configGeneration.Add(1)
	lastReloadOK.Store(true)
	lastReloadAtUnixNano.Store(time.Now().UnixNano())
}

// NoteReloadFailure 在 reloadConfig 返回错误时调用。logging.Open 失败不算。
func NoteReloadFailure() {
	reloadFailedTotal.Add(1)
	lastReloadOK.Store(false)
	lastReloadAtUnixNano.Store(time.Now().UnixNano())
}

// NoteStopTimeout 在 drainConnections 走到超时分支时加一。
func NoteStopTimeout() {
	stopTimeoutTotal.Add(1)
}

// StartedAt 是 RecordProcessStart 记下的时刻；尚未记录则为零值。
func StartedAt() time.Time {
	n := startedAtUnixNano.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// PID 是启动时的进程号。
func PID() int {
	return int(pid.Load())
}

// Version 是启动时读到的 config.CURRENT_VERSION。
func Version() string {
	v, _ := version.Load().(string)
	return v
}

// Generation 是成功 reload 的次数，进程启动为 0。
func Generation() uint64 {
	return configGeneration.Load()
}

// LastReloadOK 是最近一次 reload 是否成功。从未 reload 时为 false。
func LastReloadOK() bool {
	return lastReloadOK.Load()
}

// LastReloadAt 是最近一次 reload（成功或失败）的时间；从未 reload 则为零值。
func LastReloadAt() time.Time {
	n := lastReloadAtUnixNano.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// ReloadFailedTotal 是 reloadConfig 失败的累计次数。
func ReloadFailedTotal() uint64 {
	return reloadFailedTotal.Load()
}

// StopTimeoutTotal 是优雅停机等到超时的累计次数。
func StopTimeoutTotal() uint64 {
	return stopTimeoutTotal.Load()
}

// ResetForTest 清空进程级原子量。计数测试之间必须调用，且不得 t.Parallel。
func ResetForTest() {
	state.Store(int32(StateStarting))
	startedAtUnixNano.Store(0)
	pid.Store(0)
	version.Store("")
	configGeneration.Store(0)
	lastReloadOK.Store(false)
	lastReloadAtUnixNano.Store(0)
	reloadFailedTotal.Store(0)
	stopTimeoutTotal.Store(0)
	acceptedTotal.Store(0)
	closedTotal.Store(0)
	active.Store(0)
	listeners.Store(nil)
	requestsTotal.Store(0)
	requestsActive.Store(0)
	parseErrorsTotal.Store(0)
	abortedTotal.Store(0)
	rejectedTotal.Store(0)
	responses2xx.Store(0)
	responses3xx.Store(0)
	responses4xx.Store(0)
	responses5xx.Store(0)
	responses502.Store(0)
	responses504.Store(0)
	responsesUnknown.Store(0)
	resetDuration()
	resetSectionsForTest()
}
