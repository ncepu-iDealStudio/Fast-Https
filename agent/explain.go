package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"fast-https/agent/model"
)

const explainSystem = "只重写故障说明，让管理员更容易读。必须只返回一个 JSON 对象，键为 explanation。" +
	"不要给出命令、步骤、配置修改或授权结论。"

// narrate 只用模型改写 Cause。失败、超时或改写里出现写操作时，保留规则原文。
func narrate(ctx context.Context, p model.Provider, d Diagnosis) (string, bool) {
	if p == nil {
		return d.Cause, false
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := p.Generate(cctx, model.Request{
		Purpose: "explain",
		System:  explainSystem,
		User:    d.Cause,
	})
	if err != nil || resp == nil {
		return d.Cause, false
	}
	var body struct {
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal(resp.Content, &body); err != nil {
		return d.Cause, false
	}
	text := strings.TrimSpace(body.Explanation)
	if text == "" || mutating(text) {
		return d.Cause, false
	}
	return text, true
}
