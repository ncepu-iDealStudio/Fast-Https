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

const (
	planVerifyText    = "重新读取 /runtime 与 upstream 名单，不自动执行"
	planOnFailureText = "停止自动尝试并保留待审批记录"
)

// planCanon 的字段顺序就是哈希输入。不要改成 map。
type planCanon struct {
	EventID   string   `json:"event_id"`
	OnFailure string   `json:"on_failure"`
	Risk      string   `json:"risk"`
	Steps     []string `json:"steps"`
	Verify    string   `json:"verify"`
}

func hashPlan(eventID, risk, verify, onFailure string, steps []string) string {
	if steps == nil {
		steps = []string{}
	}
	raw, _ := json.Marshal(planCanon{
		EventID:   eventID,
		OnFailure: onFailure,
		Risk:      risk,
		Steps:     steps,
		Verify:    verify,
	})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func makePlan(taskID, eventID, classification string, steps []string) Plan {
	if steps == nil {
		steps = []string{}
	}
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
		Verify:    planVerifyText,
		OnFailure: planOnFailureText,
		Hash:      hashPlan(eventID, risk, planVerifyText, planOnFailureText, steps),
	}
}

type upstreamFact struct {
	Address string `json:"address"`
	Healthy *bool  `json:"healthy,omitempty"`
	Health  string `json:"health"`
	Drained bool   `json:"drained"`
}

// drainStep 只在一个地址当前不健康、且另有健康地址时提出摘除。
// unknown 既不算可摘除，也不算备用。已经 drained 的地址不再生成计划。
func drainStep(classification, contextJSON string) []string {
	if classification != "http_5xx_spike" && classification != "port_unreachable" {
		return nil
	}
	rows := upstreamFacts(contextJSON)
	if len(rows) < 2 {
		return nil
	}
	var bad, good []string
	for _, item := range rows {
		if item.Address == "" || item.Drained {
			continue
		}
		switch interpretHealth(item.Health, item.Healthy) {
		case "healthy":
			good = append(good, item.Address)
		case "unhealthy":
			bad = append(bad, item.Address)
		}
	}
	if len(bad) != 1 || len(good) < 1 {
		return nil
	}
	return []string{"upstream drain " + bad[0]}
}

func upstreamFacts(contextJSON string) []upstreamFact {
	var doc struct {
		Runtime json.RawMessage `json:"runtime"`
	}
	if json.Unmarshal([]byte(contextJSON), &doc) != nil || len(doc.Runtime) == 0 || doc.Runtime[0] != '{' {
		return nil
	}
	var snap struct {
		Upstreams []upstreamFact `json:"upstreams"`
		Upstream  struct {
			Upstreams []upstreamFact `json:"upstreams"`
		} `json:"upstream"`
	}
	if json.Unmarshal(doc.Runtime, &snap) != nil {
		return nil
	}
	if len(snap.Upstream.Upstreams) > 0 {
		return snap.Upstream.Upstreams
	}
	return snap.Upstreams
}
