package safe

import (
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"sync"
	"time"
)

const defaultTimeLength = 1

var Gcl []*CountLimit

type countIP struct {
	curr   int //现在的开始时间
	expire int //计数预计结束时间
	count  int //计数
	rate   int //最多允许的请求个数
}

type CountLimit struct {
	size int
	rate int
	num  int
	mu   sync.RWMutex
	gMap map[string]countIP
}

func countsInit() {
	// Gcl = *NewCountLimit(0, config.GConfig.Servers[0].Path[0].Limit.Rate)
	for _, item := range listener.GLisinfos {
		for _, path := range item.Cfg {
			tempCountLimit := NewCountLimit(0, path.Limit.Rate, path.Limit.Size*1024*1024/16)
			Gcl = append(Gcl, tempCountLimit)
		}
	}
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
		l.count++
		// fmt.Println("重置计数器之后", ipstr, "插入成功，现在count：", l.count)

		return true
	} else {
		if l.count == l.rate {
			//达到请求个数限制
			//拒绝访问
			// fmt.Println(ipstr, "拒绝插入")
			return false
		} else {
			//没有达到速率限制，计数加1
			l.count++
			// fmt.Println(ipstr, "插入成功，现在count：", l.count)
			return true
		}

	}
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
	// 获取 gMap 中的 countIP 结构体值
	cl.mu.Lock()
	a, ok := cl.gMap[ipstr]

	var flag bool
	if cl.num > cl.size {
		cl.mu.Unlock()
		return true
	}

	if ok {
		if a.Allow(ipstr) {
			flag = true
		} else {
			flag = false
		}
		cl.gMap[ipstr] = a
	} else {

		// 如果键不存在，初始化 countIP 结构体值
		cl.gMap[ipstr] = countIP{int(time.Now().Unix()),
			int(time.Now().Unix()) + defaultTimeLength, 0, cl.rate}
		a = cl.gMap[ipstr] // 获取新添加的 countIP 结构体值的引用
		cl.num++
		cl.mu.Unlock()
		return true
	}

	// 进行其他操作
	cl.mu.Unlock()
	return flag
}

func CountHandler(rr core.RRcircle) {
	core.Log(&rr.Ev.Log, rr.Ev, "")
	// message.PrintSafe(rr.Ev.Conn.RemoteAddr().String(),
	// 	" INFORMAL Event(too many)"+rr.Ev.Log,
	// 	"\""+rr.Ev.RR.Req_.Headers["User-Agent"]+"\"")
	rr.Ev.RR.Res = response.DefaultTooMany()
	rr.Ev.WriteResponseClose(nil)
}
