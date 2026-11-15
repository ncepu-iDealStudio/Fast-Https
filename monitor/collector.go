package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SchemaVersion 是 Server 在 /runtime 里报的快照版本。
// 不认识的版本按采集失败处理：宁可报错，也不按错的字段硬解析。
const SchemaVersion = "1"

// maxSnapshotBytes 限制单次快照的读取量，异常响应不该把 Monitor 的内存读爆。
const maxSnapshotBytes = 1 << 20

// FailKind 区分采集失败的类别。连不上、非 200、JSON 坏、版本不认识
// 是四种不同的坏法，混成一个「失败」会让排障无从下手。
type FailKind int

const (
	CollectOK FailKind = iota
	FailUnreachable
	FailStatus
	FailJSON
	FailSchema
)

func (k FailKind) String() string {
	switch k {
	case CollectOK:
		return "ok"
	case FailUnreachable:
		return "unreachable"
	case FailStatus:
		return "bad_status"
	case FailJSON:
		return "bad_json"
	case FailSchema:
		return "unsupported_schema"
	}
	return "unknown"
}

// Snapshot 是 Monitor 侧对 /runtime 的最小复制。
// 它只声明自己要用的字段，不依赖 runtime/metrics，也不跟着 Server 的结构体走。
type Snapshot struct {
	SchemaVersion string       `json:"schema_version"`
	ObservedAt    string       `json:"observed_at"`
	Server        ServerInfo   `json:"server"`
	Requests      RequestsInfo `json:"requests"`
	Connections   struct {
		Active   int64  `json:"active"`
		Accepted uint64 `json:"accepted_total"`
	} `json:"connections"`
	Responses5xx uint64         `json:"responses_5xx_total"`
	Listeners    []ListenerInfo `json:"listeners"`
}

// ServerInfo 是快照里的 server 对象中 Monitor 用到的部分。
type ServerInfo struct {
	State     string `json:"state"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	StartedAt string `json:"started_at"`
}

// RequestsInfo 是请求计数的两个值：累计用于规则，活动数用于疑似卡死提示。
type RequestsInfo struct {
	Active int64  `json:"active"`
	Total  uint64 `json:"total"`
}

// ListenerInfo 是实际监听的端口副本。Port 是 Server 报的串，原样保留。
type ListenerInfo struct {
	Port string `json:"port"`
	SSL  bool   `json:"ssl"`
	H2   bool   `json:"h2"`
}

// Observation 是一次采集的结果。Raw 只在成功时有值，用于原样追加到快照文件。
type Observation struct {
	At       time.Time
	Kind     FailKind
	Err      error
	Raw      json.RawMessage
	Snapshot Snapshot
}

// Collector 只做一件事：读一次 /runtime 并分类结果。它不判定事件，也不写盘。
type Collector struct {
	URL     string
	Timeout time.Duration
	// Client 供测试注入；为 nil 时用带超时的默认客户端。
	Client *http.Client
}

func (c *Collector) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{}
}

// Collect 采一次快照。ctx 之外再套一层超时，避免一个卡住的连接拖住整轮。
func (c *Collector) Collect(ctx context.Context, at time.Time) Observation {
	obs := Observation{At: at}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		obs.Kind = FailUnreachable
		obs.Err = err
		return obs
	}
	resp, err := c.client().Do(req)
	if err != nil {
		obs.Kind = FailUnreachable
		obs.Err = err
		return obs
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSnapshotBytes))
	if err != nil {
		obs.Kind = FailUnreachable
		obs.Err = fmt.Errorf("read snapshot body: %w", err)
		return obs
	}
	if resp.StatusCode != http.StatusOK {
		obs.Kind = FailStatus
		obs.Err = fmt.Errorf("snapshot returned HTTP %d", resp.StatusCode)
		return obs
	}

	var snap Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		obs.Kind = FailJSON
		obs.Err = fmt.Errorf("snapshot is not valid JSON: %w", err)
		return obs
	}
	if snap.SchemaVersion != SchemaVersion {
		obs.Kind = FailSchema
		obs.Err = fmt.Errorf("snapshot schema_version %q is not supported (want %q)", snap.SchemaVersion, SchemaVersion)
		return obs
	}

	raw, err := compactJSON(body)
	if err != nil {
		obs.Kind = FailJSON
		obs.Err = err
		return obs
	}
	obs.Kind = CollectOK
	obs.Snapshot = snap
	obs.Raw = raw
	return obs
}

// compactJSON 把快照压成一行，保证 JSONL 文件里一条记录只占一行。
func compactJSON(raw []byte) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, fmt.Errorf("snapshot is not valid JSON: %w", err)
	}
	return json.RawMessage(buf.Bytes()), nil
}
