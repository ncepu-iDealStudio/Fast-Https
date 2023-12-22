package safe

import (
	"fast-https/modules/core"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"time"
)

const defaultTimeLength = 1

var Gcl = NewCountLimit(1000, 5)

type CountLimit struct {
	rate int
	num  int
	gMap map[string]countIP
}

type countIP struct {
	curr   int //现在的开始时间
	expire int //计数预计结束时间
	count  int //计数
	rate   int //最多允许的请求个数
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

func NewCountLimit(num int, rate int) *CountLimit {
	return &CountLimit{
		gMap: make(map[string]countIP),
		num:  num,
		rate: rate,
	}
}

func (cl *CountLimit) Insert1(ipstr string) bool {
	// 获取 gMap 中的 countIP 结构体值
	a, ok := cl.gMap[ipstr]
	var flag bool
	if cl.num > 10000 {
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
		cl.gMap[ipstr] = countIP{int(time.Now().Unix()), int(time.Now().Unix()) + defaultTimeLength, 0, cl.rate}
		a = cl.gMap[ipstr] // 获取新添加的 countIP 结构体值的引用
		cl.num++
		return true
	}

	// 进行其他操作
	return flag
}

func CountHandler(rr core.RRcircle) {
	message.PrintWarn(rr.Ev.Conn.RemoteAddr().String(), "INFORMAL Event"+rr.Ev.Log,
		"\""+rr.Ev.RR.Req_.Headers["User-Agent"]+"\"")
	rr.Ev.Write_bytes_close(response.Default_too_many())
}
