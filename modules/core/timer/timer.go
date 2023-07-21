package timer

import (
	"fmt"
	"time"
)

type Timer struct {
	callback func()
	duration time.Duration
	timer    *time.Timer
}

func NewTimer(duration time.Duration, callback func()) *Timer {
	return &Timer{
		callback: callback,
		duration: duration,
		timer:    nil,
	}
}

func (t *Timer) startTimer() {
	t.timer = time.AfterFunc(t.duration, func() {
		t.callback()
	})
}

func SetTimer(duration time.Duration, callback func()) *Timer {
	timer := NewTimer(duration, callback)
	timer.startTimer()
	return timer
}

func DeleteTimer(timer *Timer) {
	timer.timer.Stop()
	fmt.Println("定时器已删除成功")
}

func UpdateTimer(timer *Timer, newDuration time.Duration) {
	timer.timer.Reset(newDuration)
	timer.duration = newDuration
	fmt.Println("定时器已更新")
}

func test() {
	timer := SetTimer(2*time.Second, func() {
		fmt.Println("定时器触发")
	})
	time.Sleep(5 * time.Second)
	DeleteTimer(timer)
	UpdateTimer(timer, 3*time.Second)
	select {}
}
