package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// eventID 与 monitor/events.go 的 eventID 使用相同输入：
// type、severity、message，以及 snapshot 的 pid 和 started_at。不含时间，不含偏移。
func eventID(ev lineEvent) string {
	if id := strings.TrimSpace(ev.EventID); validEventID(id) {
		return id
	}
	h := sha256.New()
	writePart := func(s string) {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	writePart(ev.Type)
	writePart(ev.Severity)
	writePart(ev.Message)
	if ev.Snapshot != nil {
		writePart(strconv.Itoa(ev.Snapshot.PID))
		writePart(ev.Snapshot.StartedAt)
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func validEventID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
