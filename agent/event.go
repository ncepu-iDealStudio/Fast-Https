package main

import "encoding/json"

// lineEvent 是 events.jsonl 里 Agent 需要的字段。其余键忽略。
type lineEvent struct {
	Time     string        `json:"time"`
	Type     string        `json:"type"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	EventID  string        `json:"event_id"`
	Snapshot *lineSnapshot `json:"snapshot"`
}

type lineSnapshot struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"started_at"`
}

func parseLine(raw string) (lineEvent, error) {
	var ev lineEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return lineEvent{}, err
	}
	return ev, nil
}
