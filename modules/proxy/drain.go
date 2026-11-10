package proxy

import "errors"

var (
	ErrLastUpstream     = errors.New("refusing to drain the last upstream")
	ErrUpstreamNotFound = errors.New("upstream address is not in the configuration")
)

// EvaluateDrain 判断摘除 addr 之后，每个包含它的 location 是否还剩至少一个地址。
// 不改配置文件。
func EvaluateDrain(groups [][]string, already []string, addr string) error {
	host := metricAddr(addr)
	if host == "" {
		return ErrUpstreamNotFound
	}
	blocked := map[string]struct{}{host: {}}
	for _, item := range already {
		if h := metricAddr(item); h != "" {
			blocked[h] = struct{}{}
		}
	}
	found := false
	for _, group := range groups {
		contains := false
		left := 0
		for _, peer := range group {
			h := metricAddr(peer)
			if h == host {
				contains = true
			}
			if _, skip := blocked[h]; !skip {
				left++
			}
		}
		if !contains {
			continue
		}
		found = true
		if left == 0 {
			return ErrLastUpstream
		}
	}
	if !found {
		return ErrUpstreamNotFound
	}
	return nil
}
