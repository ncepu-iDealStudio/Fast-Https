package logquery

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fast-https/infra/logging"
)

// FilterError 是调用方传入的过滤条件不在白名单语法里。
// 命令层把它映射成 USAGE_ERROR，不把日志内容算进错误。
type FilterError struct{ Msg string }

func (e *FilterError) Error() string { return e.Msg }

func badFilter(format string, args ...any) error {
	return &FilterError{Msg: fmt.Sprintf(format, args...)}
}

// statusFilter 是 --status 的三种写法：精确码、闭区间、或者 4xx 这种类。
type statusFilter struct {
	active bool
	lo, hi int
}

func parseStatus(s string) (statusFilter, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return statusFilter{}, nil
	}
	if len(s) == 3 && s[1] == 'x' && s[2] == 'x' && s[0] >= '1' && s[0] <= '5' {
		base := int(s[0]-'0') * 100
		return statusFilter{active: true, lo: base, hi: base + 99}, nil
	}
	if strings.Contains(s, "-") {
		a, b, _ := strings.Cut(s, "-")
		lo, err1 := strconv.Atoi(strings.TrimSpace(a))
		hi, err2 := strconv.Atoi(strings.TrimSpace(b))
		if err1 != nil || err2 != nil || lo < 100 || hi > 599 || lo > hi {
			return statusFilter{}, badFilter("invalid --status %q (want 500, 4xx, or 400-499)", s)
		}
		return statusFilter{active: true, lo: lo, hi: hi}, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 100 || n > 599 {
		return statusFilter{}, badFilter("invalid --status %q (want 500, 4xx, or 400-499)", s)
	}
	return statusFilter{active: true, lo: n, hi: n}, nil
}

func (f statusFilter) match(code int, ok bool) bool {
	if !f.active {
		return true
	}
	if !ok {
		return false
	}
	return code >= f.lo && code <= f.hi
}

// parseBound 把 --since / --until 收成可与日志 time 字段直接比较的字符串。
// 时长（1h、30m）按 now 往前推；已经是 RFC3339 的按原时刻重写成日志格式。
// 两种结果都带毫秒和时区，字典序等于时间序。
func parseBound(s string, now time.Time) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return "", badFilter("duration must not be negative: %q", s)
		}
		return now.Add(-d).Format(logging.TimeFormat), nil
	}
	for _, layout := range []string{logging.TimeFormat, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(logging.TimeFormat), nil
		}
	}
	return "", badFilter("invalid duration or time %q (want 1h, 30m, or RFC3339)", s)
}

func matchExact(filter, value string) bool {
	if filter == "" {
		return true
	}
	return strings.EqualFold(value, filter)
}

func matchPrefix(prefix, value string) bool {
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(value, prefix)
}

func matchTime(since, until, ts string, hasTime bool) bool {
	if since == "" && until == "" {
		return true
	}
	if !hasTime {
		return false
	}
	if since != "" && ts < since {
		return false
	}
	if until != "" && ts > until {
		return false
	}
	return true
}
