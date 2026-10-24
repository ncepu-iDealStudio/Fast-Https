// Package safe 实现限流、临时黑名单与连接计数。
package safe

func Init() {
	countsInit()
	limitInit()
	blacklistInit()
}
