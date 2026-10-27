package proxy

import (
	"sync"
	"time"

	"fast-https/infra/logging"
)

// upstreamPool 复用到同一个上游的空闲连接。
//
// 代理请求的延迟大头是每请求一次的 TCP/TLS 握手：客户端侧的
// keep-alive 早已复用，上游侧此前每个请求都重新 net.Dial。
// 池按"归一化后的上游地址"分组；借出的连接在请求结束后归还，
// 空闲超过 idleTTL 或超出 maxIdle 的连接被关闭。
var upstreamPoolInstance = newConnPool(8, 30*time.Second)

func upstreamPool() *connPool { return upstreamPoolInstance }

// timeNow 仅供测试注入时钟。
var timeNow = time.Now

type pooledIdle struct {
	p        *Proxy
	lastUsed time.Time
}

type connPool struct {
	mu      sync.Mutex
	idle    map[string][]*pooledIdle
	maxIdle int
	idleTTL time.Duration
	sweep   sync.Once
}

func newConnPool(maxIdle int, idleTTL time.Duration) *connPool {
	cp := &connPool{
		idle:    make(map[string][]*pooledIdle),
		maxIdle: maxIdle,
		idleTTL: idleTTL,
	}
	return cp
}

// get 借一条到 addr 的空闲连接；没有就 dial。空闲过久的连接上游
// 可能已经关闭，直接丢弃。
func (cp *connPool) get(addr string, dial func() (*Proxy, error)) (*Proxy, error) {
	cp.startSweeper()
	cp.mu.Lock()
	for len(cp.idle[addr]) > 0 {
		last := len(cp.idle[addr]) - 1
		pc := cp.idle[addr][last]
		cp.idle[addr] = cp.idle[addr][:last]
		cp.mu.Unlock()

		if timeNow().Sub(pc.lastUsed) < cp.idleTTL {
			pc.p.reused = true
			return pc.p, nil
		}
		pc.p.Close()
		cp.mu.Lock()
	}
	cp.mu.Unlock()
	return dial()
}

// put 归还一条健康连接。池已满时直接关闭。
func (cp *connPool) put(addr string, p *Proxy) {
	if p == nil || p.isClosed() {
		return
	}
	cp.mu.Lock()
	if len(cp.idle[addr]) >= cp.maxIdle {
		cp.mu.Unlock()
		p.Close()
		return
	}
	cp.idle[addr] = append(cp.idle[addr], &pooledIdle{p: p, lastUsed: timeNow()})
	cp.mu.Unlock()
}

// startSweeper 周期性关闭超过空闲时长的连接，避免不再有流量的
// 上游把句柄一直挂在池里。
func (cp *connPool) startSweeper() {
	cp.sweep.Do(func() {
		go func() {
			ticker := time.NewTicker(cp.idleTTL / 2)
			defer ticker.Stop()
			for range ticker.C {
				now := timeNow()
				var expired []*Proxy
				cp.mu.Lock()
				for addr, conns := range cp.idle {
					kept := conns[:0]
					for _, pc := range conns {
						if now.Sub(pc.lastUsed) >= cp.idleTTL {
							expired = append(expired, pc.p)
						} else {
							kept = append(kept, pc)
						}
					}
					if len(kept) == 0 {
						delete(cp.idle, addr)
					} else {
						cp.idle[addr] = kept
					}
				}
				cp.mu.Unlock()
				for _, p := range expired {
					p.Close()
				}
				if len(expired) > 0 {
					logging.ConsoleLog.Debug().Msgf("proxy pool closed %d expired idle connections", len(expired))
				}
			}
		}()
	})
}
