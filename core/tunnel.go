package core

import (
	"sync/atomic"
	"time"

	"fast-https/config"
)

// TunnelIdle 跟踪一条双向隧道（websocket / tcp proxy）的最后活动时间。
//
// 两条转发协程共享同一个实例：任意方向读到数据都会 Touch。某次读超时后
// 用 Expired 判断是「真的空闲」还是「另一个方向仍在传数据」——后者只重新
// 挂 deadline，不关闭连接。timeout 为 0 表示不启用空闲断开。
type TunnelIdle struct {
	timeout time.Duration
	last    atomic.Int64
}

// NewTunnelIdle 按当前配置创建空闲跟踪器；0 表示不启用。
func NewTunnelIdle() *TunnelIdle {
	t := &TunnelIdle{timeout: config.Current().Proxy.TunnelIdleTimeoutDuration()}
	t.Touch()
	return t
}

// Enabled 报告是否配置了空闲断开。
func (t *TunnelIdle) Enabled() bool { return t != nil && t.timeout > 0 }

// Touch 记录一次任意方向的数据活动。
func (t *TunnelIdle) Touch() {
	if t != nil {
		t.last.Store(time.Now().UnixNano())
	}
}

// Deadline 返回本次读应设置的 deadline；未启用时返回零值（不设 deadline）。
func (t *TunnelIdle) Deadline() time.Time {
	if !t.Enabled() {
		return time.Time{}
	}
	return time.Now().Add(t.timeout)
}

// Expired 报告自最后一次活动起是否已超过空闲上限。
func (t *TunnelIdle) Expired() bool {
	if !t.Enabled() {
		return false
	}
	return time.Since(time.Unix(0, t.last.Load())) >= t.timeout
}
