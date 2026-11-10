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

func makePlan(taskID, eventID, classification string) Plan {
	steps := []string{}
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
		Verify:    "重新读取 /runtime 与 monitor status，不执行写命令",
		OnFailure: "停止自动尝试并保留待审批或升级记录",
		Hash:      hex.EncodeToString(sum[:]),
	}
}
