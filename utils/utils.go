package utils

import (
	"regexp"
	"strconv"
	"sync"
	"time"
)

var waitGroup sync.WaitGroup

func GetWaitGroup() *sync.WaitGroup {
	return &waitGroup
}

// ParseTime
// @Author yizhigopher
// @Description 将1d20h30m10s字符串转化为具体的秒数
func ParseTime(timeStr string) int {
	expect := []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second}
	re := regexp.MustCompile(`(?:([0-9]+)d)?(?:([0-9]+)h)?(?:([0-9]+)m)?(?:([0-9]+)s)?`)
	matches := re.FindStringSubmatch(timeStr)
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
