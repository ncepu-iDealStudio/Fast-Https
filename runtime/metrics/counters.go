package metrics

import "sync/atomic"

var (
	acceptedTotal atomic.Uint64
	closedTotal   atomic.Uint64
	active        atomic.Int64
)

// ConnAccepted 在 serveConn 确认不是停机丢弃之后调用：接受数和活动数各加一。
func ConnAccepted() {
	acceptedTotal.Add(1)
	active.Add(1)
}

// ConnClosed 在处理协程的 defer 里调用，覆盖提前返回和 panic。
func ConnClosed() {
	closedTotal.Add(1)
	active.Add(-1)
}

// ConnAcceptedTotal 是进程启动以来接受过的连接数。
func ConnAcceptedTotal() uint64 {
	return acceptedTotal.Load()
}

// ConnClosedTotal 是进程启动以来已结束的连接数。
func ConnClosedTotal() uint64 {
	return closedTotal.Load()
}

// ConnActive 是当前仍在处理的连接数。
func ConnActive() int64 {
	return active.Load()
}
