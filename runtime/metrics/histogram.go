package metrics

import (
	"sync/atomic"
	"time"
)

// 固定桶上界：1ms、5ms、10ms、25ms、50ms、100ms、250ms、500ms、1s、5s，另加 +Inf。
// 计数是累积的：桶 le=N 表示耗时 ≤ N 的请求数。
var durationBounds = [...]time.Duration{
	time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	5 * time.Second,
}

const durationInfIndex = len(durationBounds)

var (
	durationCount   atomic.Uint64
	durationBuckets [durationInfIndex + 1]atomic.Uint64
)

// DurationHistogram 是 /runtime 的 request_duration 对象，单位毫秒。
type DurationHistogram struct {
	Unit    string           `json:"unit"`
	Count   uint64           `json:"count"`
	Buckets []DurationBucket `json:"buckets"`
}

// DurationBucket 是一个累积桶。Le 为毫秒数（JSON number）或 "+Inf"。
type DurationBucket struct {
	Le    any    `json:"le"`
	Count uint64 `json:"count"`
}

// ObserveDuration 把一次 EventHandler 耗时打进累积直方图。
func ObserveDuration(d time.Duration) {
	if d < 0 {
		d = 0
	}
	durationCount.Add(1)
	for i, bound := range durationBounds {
		if d <= bound {
			durationBuckets[i].Add(1)
		}
	}
	durationBuckets[durationInfIndex].Add(1)
}

// DurationSnapshot 返回当前耗时直方图。count 为 0 是真实的零，不是未知。
func DurationSnapshot() DurationHistogram {
	h := DurationHistogram{
		Unit:    "ms",
		Count:   durationCount.Load(),
		Buckets: make([]DurationBucket, 0, durationInfIndex+1),
	}
	for i, bound := range durationBounds {
		h.Buckets = append(h.Buckets, DurationBucket{
			Le:    float64(bound.Milliseconds()),
			Count: durationBuckets[i].Load(),
		})
	}
	h.Buckets = append(h.Buckets, DurationBucket{
		Le:    "+Inf",
		Count: durationBuckets[durationInfIndex].Load(),
	})
	return h
}

func resetDuration() {
	durationCount.Store(0)
	for i := range durationBuckets {
		durationBuckets[i].Store(0)
	}
}
