// Package rtclient 读取本机 GET /runtime 快照。只使用标准库 HTTP，不导入 runtime/metrics。
package rtclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"fast-https/infra/logging"
)

const (
	SchemaVersion   = "1"
	EnvURL          = "FASTHTTPS_RUNTIME_URL"
	DefaultEndpoint = "http://127.0.0.1:9088/runtime"
	DefaultTimeout  = time.Second
)

var (
	ErrUnreachable = errors.New("runtime snapshot unreachable")
	ErrSchema      = errors.New("runtime snapshot schema is not supported")
)

var defaultHTTP = &http.Client{Timeout: DefaultTimeout}

// Client 拉取本机运行快照。
type Client struct {
	URL  string
	HTTP *http.Client
}

// DefaultURL 是当前进程要请求的快照地址。测试可用 FASTHTTPS_RUNTIME_URL 覆盖。
func DefaultURL() string {
	if v := strings.TrimSpace(os.Getenv(EnvURL)); v != "" {
		return v
	}
	return DefaultEndpoint
}

// Default 返回指向 DefaultURL 的客户端，超时 1 秒。
func Default() Client {
	return Client{URL: DefaultURL(), HTTP: defaultHTTP}
}

// Get 请求快照。schema_version 必须为 "1"。
func (c Client) Get(ctx context.Context) (Snapshot, error) {
	url := strings.TrimSpace(c.URL)
	if url == "" {
		url = DefaultURL()
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = defaultHTTP
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("%w: HTTP %d", ErrUnreachable, resp.StatusCode)
	}
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrSchema, err)
	}
	if snap.SchemaVersion != SchemaVersion {
		return Snapshot{}, fmt.Errorf("%w: got %q", ErrSchema, snap.SchemaVersion)
	}
	return snap, nil
}

// Snapshot 是 /runtime 的客户端视图。字段名与服务端 JSON 对齐；不认识的字段忽略。
type Snapshot struct {
	SchemaVersion    string             `json:"schema_version"`
	ObservedAt       string             `json:"observed_at"`
	Server           ServerInfo         `json:"server"`
	Listeners        []ListenerInfo     `json:"listeners,omitempty"`
	Connections      ConnectionsInfo    `json:"connections"`
	Requests         RequestsInfo       `json:"requests"`
	Responses2xx     uint64             `json:"responses_2xx_total"`
	Responses3xx     uint64             `json:"responses_3xx_total"`
	Responses4xx     uint64             `json:"responses_4xx_total"`
	Responses5xx     uint64             `json:"responses_5xx_total"`
	Responses502     uint64             `json:"responses_502_total"`
	Responses504     uint64             `json:"responses_504_total"`
	ResponsesUnknown uint64             `json:"responses_unknown_total"`
	RequestDuration  *DurationHistogram `json:"request_duration,omitempty"`
	Upstream         *UpstreamInfo      `json:"upstream,omitempty"`
}

// ServerInfo 是快照里的 server 对象。
type ServerInfo struct {
	State             string `json:"state"`
	PID               int    `json:"pid,omitempty"`
	Version           string `json:"version,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	ConfigGeneration  uint64 `json:"config_generation"`
	LastReloadOK      *bool  `json:"last_reload_ok,omitempty"`
	LastReloadAt      string `json:"last_reload_at,omitempty"`
	ReloadFailedTotal uint64 `json:"reload_failed_total"`
	StopTimeoutTotal  uint64 `json:"stop_timeout_total"`
}

// ListenerInfo 是当前实际监听的端口。
type ListenerInfo struct {
	Port string `json:"port"`
	SSL  bool   `json:"ssl,omitempty"`
	H2   bool   `json:"h2,omitempty"`
}

// ConnectionsInfo 是连接计数。
type ConnectionsInfo struct {
	Active        int64  `json:"active"`
	AcceptedTotal uint64 `json:"accepted_total"`
	ClosedTotal   uint64 `json:"closed_total"`
	RejectedTotal uint64 `json:"rejected_total"`
}

// RequestsInfo 是请求计数。
type RequestsInfo struct {
	Active           int64  `json:"active"`
	Total            uint64 `json:"total"`
	ParseErrorsTotal uint64 `json:"parse_errors_total"`
	AbortedTotal     uint64 `json:"aborted_total"`
}

// DurationHistogram 是 request_duration，单位毫秒。
type DurationHistogram struct {
	Unit    string           `json:"unit"`
	Count   uint64           `json:"count"`
	Buckets []DurationBucket `json:"buckets"`
}

// DurationBucket 是累积桶。Le 为毫秒数或 "+Inf"。
type DurationBucket struct {
	Le    any    `json:"le"`
	Count uint64 `json:"count"`
}

// UpstreamInfo 是代理上游计数。未挂入时整段省略。
// 四个总和覆盖全部地址；upstreams 最多列出 64 个 host:port，不含用户信息和路径。
type UpstreamInfo struct {
	DialFailTotal    uint64             `json:"dial_fail_total"`
	SendTimeoutTotal uint64             `json:"send_timeout_total"`
	ReadTimeoutTotal uint64             `json:"read_timeout_total"`
	IdleConns        int64              `json:"idle_conns"`
	Upstreams        []UpstreamAddrInfo `json:"upstreams,omitempty"`
}

// UpstreamAddrInfo 是一个上游地址上的失败计数。
type UpstreamAddrInfo struct {
	Address          string `json:"address"`
	DialFailTotal    uint64 `json:"dial_fail_total"`
	SendTimeoutTotal uint64 `json:"send_timeout_total"`
	ReadTimeoutTotal uint64 `json:"read_timeout_total"`
}

// UptimeSeconds 由 started_at 推算。解析失败则省略。
func (s ServerInfo) UptimeSeconds(now time.Time) *int64 {
	if s.StartedAt == "" {
		return nil
	}
	at, err := time.Parse(logging.TimeFormat, s.StartedAt)
	if err != nil {
		at, err = time.Parse(time.RFC3339, s.StartedAt)
		if err != nil {
			return nil
		}
	}
	sec := int64(now.Sub(at).Seconds())
	if sec < 0 {
		sec = 0
	}
	return &sec
}
