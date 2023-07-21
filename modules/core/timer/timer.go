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

func Initialize() {
	fmt.Println("定时器模块已初始化")
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
	Initialize()

	// 设置一个定时器
	timer := SetTimer(2*time.Second, func() {
		fmt.Println("定时器触发")
	})

	// 等待一段时间
	time.Sleep(5 * time.Second)

	// 删除定时器
	DeleteTimer(timer)

	// 设置新的定时器
	UpdateTimer(timer, 3*time.Second)

	select {}
}
