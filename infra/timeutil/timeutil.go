// Package timeutil 解析 nginx 风格的时长字符串。
//
// 该包只依赖标准库，属于基础设施层，不允许引入配置或业务包。
package timeutil

import (
	"regexp"
	"strconv"
	"time"
)

var durationPattern = regexp.MustCompile(`(?:([0-9]+)d)?(?:([0-9]+)h)?(?:([0-9]+)m)?(?:([0-9]+)s)?`)

// ParseTime 将 "1d20h30m10s" 形式的时长字符串转换为秒数。
func ParseTime(timeStr string) int {
	expect := []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second}
	matches := durationPattern.FindStringSubmatch(timeStr)
	duration := time.Duration(0)
	for i, d := range matches {
		if i == 0 {
			continue
		}
		temp, _ := strconv.Atoi(d)
		duration += time.Duration(temp) * expect[i-1]
	}
	return int(duration.Seconds())
}
