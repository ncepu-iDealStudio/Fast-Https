package safe

import (
	"fmt"
	"golang.org/x/time/rate"
	"time"
)

func BucketTest1() {
	limit := rate.Every(300 * time.Millisecond)
	limiter := rate.NewLimiter(limit, 5)

	for i := 0; i < 10; i++ {
		// 检查是否允许进行下一个事件
		if limiter.Allow() {
			fmt.Println("允许插入", i+1)
		} else {
			fmt.Println("拒绝插入", i+1)
		}

		// 模拟事件处理时间
		time.Sleep(time.Millisecond * 100)
	}
}
