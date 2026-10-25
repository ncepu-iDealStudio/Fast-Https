package safe

import (
	"fast-https/core"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
	"sync"
	"sync/atomic"
	"time"
)

const defaultTimeLength = 1

// Replaced as a whole on reload; requests in flight keep reading the old table.
var gcl atomic.Pointer[[]*CountLimit]

type countIP struct {
	curr   int //现在的开始时间
	expire int //计数预计结束时间
	count  int //计数
	rate   int //最多允许的请求个数
}

type CountLimit struct {
	size      int
	rate      int
	num       int
	mu        sync.RWMutex
	gMap      map[string]countIP
	lastSweep int // 上次过期清扫的 Unix 秒，每秒至多一次
}

func countsInit() {
	size := 0
	for _, item := range listener.GLisinfos {
		for _, path := range item.Cfg {
			if path.ID >= size {
				size = path.ID + 1
			}
		}
	}
	next := make([]*CountLimit, size)
	for _, item := range listener.GLisinfos {
		for _, path := range item.Cfg {
			if path.ID >= 0 {
				next[path.ID] = NewCountLimit(0, path.Limit.Rate, path.Limit.Size*1024*1024/16)
			}
		}
	}
	gcl.Store(&next)
}

// CountLimitFor returns nil when id is not in the current table,
// e.g. a request matched before reload shrank the location list.
func CountLimitFor(id int) *CountLimit {
	table := gcl.Load()
	if table == nil || id < 0 || id >= len(*table) {
		return nil
	}
	return (*table)[id]
}

func (l *countIP) Set(r int, now time.Time) {
	l.rate = r
	l.curr = int(now.Unix())
	l.expire = int(now.Unix()) + defaultTimeLength
	l.count = 0
}

func (l *countIP) Reset(now time.Time) {
	l.curr = int(now.Unix())
	l.expire = int(now.Unix()) + defaultTimeLength
	l.count = 0
}

func (l *countIP) Allow(ipstr string) bool {

	now := time.Now()
	if int(now.Unix()) >= l.expire {
		// 到时间了 重置计数器
		l.Reset(now)
	}
	// 先比较后自增：rate 次就是 rate 次。
	// 旧实现先判断后自增，实际放行 rate+1 次。
	if l.count >= l.rate {
		//达到请求个数限制
		//拒绝访问
		return false
	}
	l.count++
	return true
}

func NewCountLimit(num int, rate int, size int) *CountLimit {
	return &CountLimit{
		gMap: make(map[string]countIP),
		num:  num,
		rate: rate,
		size: size,
	}
}

func (cl *CountLimit) Insert(ipstr string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	now := int(time.Now().Unix())
	cl.sweepExpired(now)

	if cl.num > cl.size {
		return true
	}

	a, ok := cl.gMap[ipstr]
	if !ok {
		// 如果键不存在，初始化 countIP 结构体值；
		// 当前请求即计数 1，与 Allow 的"先比较后自增"语义一致
		cl.gMap[ipstr] = countIP{now, now + defaultTimeLength, 1, cl.rate}
		cl.num++
		return true
	}

	flag := a.Allow(ipstr)
	cl.gMap[ipstr] = a
	return flag
}

// sweepExpired 删除不活跃 IP 的条目，防止 map 无界增长：
// 窗口只有 1 秒，过期的条目下次命中也会被重置，删除与重置等价。
func (cl *CountLimit) sweepExpired(now int) {
	if now == cl.lastSweep {
		return
	}
	cl.lastSweep = now
	for key, c := range cl.gMap {
		if now >= c.expire {
			delete(cl.gMap, key)
			cl.num--
		}
	}
}

func CountHandler(rr core.RRcircle) {
	log := dynlog.DynLogger{}
	dynlog.Log(&log, rr.Ev)
	logReject(rr.Ev, ReasonLocationLimit)
	rr.Ev.RR.Res = response.DefaultTooMany()
	rr.Ev.WriteResponseClose(nil)
}
