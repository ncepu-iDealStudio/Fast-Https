package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Chat 调用 OpenAI 兼容的 /chat/completions。不注册工具。
type Chat struct {
	BaseURL string
	Name    string
	Model   string
	APIKey  string
	HTTP    *http.Client
}

func (c Chat) Generate(ctx context.Context, req Request) (*Response, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" || c.Model == "" {
		return nil, ErrUnavailable
	}
	payload := map[string]any{
		"model": c.Model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("model response has no choices")
	}
	return &Response{
		Provider: c.Name,
		Model:    c.Model,
		Content:  json.RawMessage(parsed.Choices[0].Message.Content),
	}, nil
}

// FromEnv 按地址组装本地优先、外部备用的路由器。都没配置时返回 nil。
func FromEnv(localURL, localModel, externalURL, externalModel, apiKey string) Provider {
	var r Router
	if strings.TrimSpace(localURL) != "" {
		r.Local = Chat{BaseURL: localURL, Name: "local", Model: first(localModel, "local"), APIKey: apiKey}
	}
	if strings.TrimSpace(externalURL) != "" {
		r.External = Chat{BaseURL: externalURL, Name: "external", Model: first(externalModel, "external"), APIKey: apiKey}
	}
	if r.Local == nil && r.External == nil {
		return nil
	}
	return r
}

func first(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
