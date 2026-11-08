package main

import "time"

// Clock 是可替换的时间来源。生产用 SystemClock，测试用假时钟，
// 这样单元测试不需要真的等 5 秒。
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SystemClock 返回读系统时间的时钟。
func SystemClock() Clock { return systemClock{} }
