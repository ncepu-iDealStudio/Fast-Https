package model

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrUnavailable 表示没有配置可用的模型。规则诊断仍可继续。
var ErrUnavailable = errors.New("model unavailable")

// Request 是一次模型调用。模型只返回结构化文本，不得到工具。
type Request struct {
	Purpose    string
	System     string
	User       string
	JSONSchema json.RawMessage
}

// Response 是模型返回的 JSON 正文。不能反序列化时视为调用失败。
type Response struct {
	Provider string
	Model    string
	Content  json.RawMessage
}

// Provider 是本地或外部模型的统一接口。
type Provider interface {
	Generate(ctx context.Context, req Request) (*Response, error)
}

// Router 先试本地，失败再试外部。两者都没有时返回 ErrUnavailable。
type Router struct {
	Local    Provider
	External Provider
}

func (r Router) Generate(ctx context.Context, req Request) (*Response, error) {
	if r.Local != nil {
		resp, err := r.Local.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
	}
	if r.External != nil {
		return r.External.Generate(ctx, req)
	}
	return nil, ErrUnavailable
}
