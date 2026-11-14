package main

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// CollectOptions 控制一次上下文采集。Run 为空时用 exec.Command。
type CollectOptions struct {
	Binary     string
	RuntimeURL string
	Run        cmdRunner
	Now        time.Time
	Ctx        context.Context
}

type contextDoc struct {
	Runtime  json.RawMessage            `json:"runtime"`
	Commands map[string]json.RawMessage `json:"commands"`
	Warnings []string                   `json:"warnings"`
}

// CollectPending 给还没有上下文的 PENDING / COLLECTING 任务写 context_json。
// 单项失败记入 warnings，不把任务标成失败。
func CollectPending(stateDir string, opt CollectOptions) (int, error) {
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	st, err := OpenStore(stateDir)
	if err != nil {
		return 0, err
	}
	defer st.Close()
	ids, err := st.tasksWithoutContext()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		ctx := opt.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		raw, err := json.Marshal(buildContext(ctx, opt))
		if err != nil {
			return n, err
		}
		if err := st.saveContext(id, string(raw), opt.Now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func buildContext(ctx context.Context, opt CollectOptions) contextDoc {
	doc := contextDoc{
		Commands: map[string]json.RawMessage{},
		Warnings: []string{},
	}
	snap, err := fetchRuntime(ctx, opt.RuntimeURL)
	if err != nil {
		doc.Runtime = json.RawMessage(`"unavailable"`)
		doc.Warnings = append(doc.Warnings, "runtime: "+err.Error())
	} else {
		doc.Runtime = snap
	}
	for _, args := range allowedCLI {
		key := strings.Join(args, " ")
		body, err := runAllowed(ctx, opt.Run, opt.Binary, args)
		if err != nil {
			doc.Warnings = append(doc.Warnings, key+": "+err.Error())
			continue
		}
		doc.Commands[key] = body
	}
	return doc
}
