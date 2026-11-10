package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatRejectsMissingChoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	_, err := (Chat{BaseURL: srv.URL, Model: "m"}).Generate(context.Background(), Request{System: "s", User: "u"})
	if err == nil {
		t.Fatal("bad status was accepted")
	}
}

func TestRouterFallsBack(t *testing.T) {
	r := Router{
		Local:    stubProvider{err: ErrUnavailable},
		External: stubProvider{content: []byte(`{"explanation":"ok"}`)},
	}
	resp, err := r.Generate(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Content) != `{"explanation":"ok"}` {
		t.Fatalf("content = %s", resp.Content)
	}
}

type stubProvider struct {
	content []byte
	err     error
}

func (s stubProvider) Generate(ctx context.Context, req Request) (*Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &Response{Content: s.content}, nil
}
