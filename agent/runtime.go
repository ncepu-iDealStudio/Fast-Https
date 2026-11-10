package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const runtimeSchema = "1"

// fetchRuntime 读取本机快照。schema_version 不是 "1" 时返回 unavailable，不把正文交给诊断。
func fetchRuntime(ctx context.Context, url string) (json.RawMessage, error) {
	if url == "" {
		url = "http://127.0.0.1:9088/runtime"
	}
	cctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runtime HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var head struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return nil, err
	}
	if head.SchemaVersion != runtimeSchema {
		return nil, fmt.Errorf("runtime schema %q", head.SchemaVersion)
	}
	return json.RawMessage(body), nil
}
