package safe

import (
	"fmt"
	"time"
)

const timeLength = 5

type countIP struct {
	curr   int //现在的开始时间
	expire int //计数预计结束时间
	count  int //计数
	rate   int //最多允许的请求个数

}

func (l *countIP) Set(r int, now time.Time) {
	l.rate = r
	l.curr = int(now.Unix())
	l.expire = int(now.Unix()) + timeLength
	l.count = 0
}

func (l *countIP) Reset(now time.Time) {
	l.curr = int(now.Unix())
	l.expire = int(now.Unix()) + timeLength
	l.count = 0
}

func (l *countIP) Allow(ipstr string) bool {

	//if l.count == l.rate-1 {
	//	//达到请求个数限制
	//	now := time.Now()
	//	if int(now.Unix()) >= l.expire {
	//		//速度允许范围内， 到时间了 重置计数器
	//		l.Reset(now)
	//		return true
	//	} else {
	//		return false
	//	}
	//} else {
	//	//没有达到速率限制，计数加1
	//
	//	l.count++
	//	return true
	//}

	now := time.Now()
	if int(now.Unix()) >= l.expire {
		// 到时间了 重置计数器
		l.Reset(now)
		l.count++
		fmt.Println("重置计数器之后", ipstr, "插入成功，现在count：", l.count)

		return true
	} else {
		if l.count == l.rate {
			//达到请求个数限制
			//拒绝访问
			fmt.Println(ipstr, "拒绝插入")
			return false

		} else {
			//没有达到速率限制，计数加1

			l.count++
			fmt.Println(ipstr, "插入成功，现在count：", l.count)
			return true
		}

	}

}

var gMap map[string]countIP

func insert1(ipstr string) {
	// 获取 gMap 中的 countIP 结构体值
	a, ok := gMap[ipstr]

	if ok {
		// 如果键存在，进行相应操作

		//if a.expire < int(time.Now().Unix()) {
		//	a.Reset(time.Now())
		//	return
		//} else {
		//	a.count++
		//	gMap[ipstr] = a // 更新 gMap 中的值
		//}
		a.Allow(ipstr)
		gMap[ipstr] = a
	} else {
		// 如果键不存在，初始化 countIP 结构体值
		gMap[ipstr] = countIP{int(time.Now().Unix()), int(time.Now().Unix()) + timeLength, 0, 10}
		a = gMap[ipstr] // 获取新添加的 countIP 结构体值的引用

	}

	// 进行其他操作

}

func TimeTest2() {
	if gMap == nil {
		gMap = make(map[string]countIP)
	}
	// i := 0
	for {
		insert1("127.0.0.1")
		insert1("127.0.0.1")
		insert1("127.0.0.2")

		time.Sleep(time.Millisecond * 300)
		//fmt.Println(gMap)
		// i++
	}
}
