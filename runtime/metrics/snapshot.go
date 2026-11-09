package metrics

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"fast-https/config"
	"fast-https/infra/logging"
)

const SchemaVersion = "1"

type section struct {
	name string
	fn   func() (any, bool)
}

var (
	sectionMu sync.Mutex
	sections  []section
)

// RegisterSection 把一个可选分段挂进快照。fn 返回 ok=false 时省略该字段。
// 组合根用它挂入 modules/proxy 的 upstream，避免 runtime 依赖功能包。
func RegisterSection(name string, fn func() (any, bool)) {
	if name == "" || fn == nil {
		return
	}
	sectionMu.Lock()
	defer sectionMu.Unlock()
	for i, s := range sections {
		if s.name == name {
			sections[i].fn = fn
			return
		}
	}
	sections = append(sections, section{name: name, fn: fn})
}

func resetSectionsForTest() {
	sectionMu.Lock()
	sections = nil
	sectionMu.Unlock()
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

// ConnectionsInfo 是连接计数。
type ConnectionsInfo struct {
	Active        int64  `json:"active"`
	AcceptedTotal uint64 `json:"accepted_total"`
	ClosedTotal   uint64 `json:"closed_total"`
	RejectedTotal uint64 `json:"rejected_total"`
}

// RequestsInfo 是请求计数。status 分桶在 Snapshot 顶层，与扫描报告示例一致。
type RequestsInfo struct {
	Active           int64  `json:"active"`
	Total            uint64 `json:"total"`
	ParseErrorsTotal uint64 `json:"parse_errors_total"`
	AbortedTotal     uint64 `json:"aborted_total"`
}

// Snapshot 是 /runtime 的可序列化结构。只 Load 原子值。
type Snapshot struct {
	SchemaVersion    string            `json:"schema_version"`
	ObservedAt       string            `json:"observed_at"`
	Server           ServerInfo        `json:"server"`
	Listeners        []ListenerInfo    `json:"listeners,omitempty"`
	Connections      ConnectionsInfo   `json:"connections"`
	Requests         RequestsInfo      `json:"requests"`
	Responses2xx     uint64            `json:"responses_2xx_total"`
	Responses3xx     uint64            `json:"responses_3xx_total"`
	Responses4xx     uint64            `json:"responses_4xx_total"`
	Responses5xx     uint64            `json:"responses_5xx_total"`
	Responses502     uint64            `json:"responses_502_total"`
	Responses504     uint64            `json:"responses_504_total"`
	ResponsesUnknown uint64            `json:"responses_unknown_total"`
	RequestDuration  DurationHistogram `json:"request_duration"`
	extra            map[string]any    `json:"-"`
}

func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	raw, err := json.Marshal(alias(s))
	if err != nil || len(s.extra) == 0 {
		return raw, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for k, v := range s.extra {
		m[k] = v
	}
	return json.Marshal(m)
}

// TakeSnapshot 组装当前进程内状态。request_duration 始终出现（零是真实的零）；
// upstream 由组合根经 RegisterSection 挂入，未注册则省略。
func TakeSnapshot() Snapshot {
	srv := ServerInfo{
		State:             State().String(),
		PID:               PID(),
		Version:           Version(),
		ConfigGeneration:  Generation(),
		ReloadFailedTotal: ReloadFailedTotal(),
		StopTimeoutTotal:  StopTimeoutTotal(),
	}
	if srv.PID == 0 {
		srv.PID = os.Getpid()
	}
	if srv.Version == "" {
		srv.Version = config.CURRENT_VERSION
	}
	if at := StartedAt(); !at.IsZero() {
		srv.StartedAt = at.Format(logging.TimeFormat)
	}
	if at := LastReloadAt(); !at.IsZero() {
		ok := LastReloadOK()
		srv.LastReloadOK = &ok
		srv.LastReloadAt = at.Format(logging.TimeFormat)
	}

	snap := Snapshot{
		SchemaVersion: SchemaVersion,
		ObservedAt:    time.Now().Format(logging.TimeFormat),
		Server:        srv,
		Listeners:     Listeners(),
		Connections: ConnectionsInfo{
			Active:        ConnActive(),
			AcceptedTotal: ConnAcceptedTotal(),
			ClosedTotal:   ConnClosedTotal(),
			RejectedTotal: RejectedTotal(),
		},
		Requests: RequestsInfo{
			Active:           RequestsActive(),
			Total:            RequestsTotal(),
			ParseErrorsTotal: ParseErrorsTotal(),
			AbortedTotal:     AbortedTotal(),
		},
		Responses2xx:     Responses2xx(),
		Responses3xx:     Responses3xx(),
		Responses4xx:     Responses4xx(),
		Responses5xx:     Responses5xx(),
		Responses502:     Responses502(),
		Responses504:     Responses504(),
		ResponsesUnknown: ResponsesUnknown(),
		RequestDuration:  DurationSnapshot(),
	}

	sectionMu.Lock()
	copySecs := append([]section(nil), sections...)
	sectionMu.Unlock()
	for _, sec := range copySecs {
		v, ok := sec.fn()
		if !ok {
			continue
		}
		if snap.extra == nil {
			snap.extra = make(map[string]any)
		}
		snap.extra[sec.name] = v
	}
	return snap
}
