package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Plan 是给策略引擎看的步骤列表。本阶段步骤为空，不包含写命令。
type Plan struct {
	ID        string
	EventID   string
	TaskID    string
	Risk      string
	Steps     []string
	Verify    string
	OnFailure string
	Hash      string
}

func makePlan(taskID, eventID, classification string, steps []string) Plan {
	if steps == nil {
		steps = []string{}
	}
	raw, _ := json.Marshal(steps)
	sum := sha256.Sum256(append([]byte(eventID+"\n"), raw...))
	risk := "low"
	if classification == "unknown" || classification == "collect_error" || classification == "certificate_read_error" {
		risk = "unknown"
	}
	return Plan{
		ID:        "p_" + eventID,
		EventID:   eventID,
		TaskID:    taskID,
		Risk:      risk,
		Steps:     steps,
		Verify:    "重新读取 /runtime 与 upstream 名单，不自动执行",
		OnFailure: "停止自动尝试并保留待审批记录",
		Hash:      hex.EncodeToString(sum[:]),
	}
}

// drainStep 只有在恰好一个地址不健康、并且还有另一个地址时，才提出摘除。
// 事件说明里的命令文本不会出现在步骤里。
func drainStep(classification, contextJSON string) []string {
	if classification != "http_5xx_spike" && classification != "port_unreachable" {
		return nil
	}
	var doc struct {
		Runtime json.RawMessage `json:"runtime"`
	}
	if json.Unmarshal([]byte(contextJSON), &doc) != nil || len(doc.Runtime) == 0 || doc.Runtime[0] != '{' {
		return nil
	}
	var snap struct {
		Upstreams []struct {
			Address string `json:"address"`
			Healthy bool   `json:"healthy"`
		} `json:"upstreams"`
	}
	if json.Unmarshal(doc.Runtime, &snap) != nil {
		return nil
	}
	var bad, good []string
	for _, item := range snap.Upstreams {
		if item.Address == "" {
			continue
		}
		if item.Healthy {
			good = append(good, item.Address)
		} else {
			bad = append(bad, item.Address)
		}
	}
	if len(bad) != 1 || len(good) < 1 {
		return nil
	}
	return []string{"upstream drain " + bad[0]}
}
