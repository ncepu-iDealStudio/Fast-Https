package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TimeFormat 与 infra/logging 的时间字段一致，CLI 的 monview 按同一布局解析。
const TimeFormat = "2006-01-02T15:04:05.000Z07:00"

// 三个落盘文件的名字与 CLI 侧 monview 的约定一致。
const (
	SnapshotsFile = "snapshots.jsonl"
	EventsFile    = "events.jsonl"
	StatusFile    = "status.json"
)

// StatusSchemaVersion 是状态文件的版本；CLI 只认这一个。
const StatusSchemaVersion = "1"

// 事件类型。规则只做确定性判断，类型名就是结论本身；事件里的 rule 与 type 同名，
// 便于按规则过滤。
const (
	EventUnreachable  = "unreachable"
	EventRecovered    = "recovered"
	EventRestarted    = "restarted"
	EventCollectError = "collect_error"
	EventFiveXXRatio  = "5xx_ratio"
	EventPortProbe    = "port_probe"
	EventStuck        = "stuck_suspected"
)

// 状态文件里的 status 取值。stale 由读侧按时间差推导，Monitor 不写。
const (
	StatusRunning   = "running"
	StatusUnhealthy = "unhealthy"
)

// Event 是一行事件 JSON。字段与 CLI 的白名单一致：只写这五项，
// 不写请求 URI、上游地址、证书路径这类内部信息。
type Event struct {
	Time     string        `json:"time"`
	Type     string        `json:"type"`
	Rule     string        `json:"rule"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	Summary  *EventSummary `json:"snapshot,omitempty"`
}

// EventSummary 是触发时的快照摘要，供事后回溯。窗口规则的增量也放在这里，
// 免得回头再算一遍。
type EventSummary struct {
	State             string `json:"state,omitempty"`
	PID               int    `json:"pid,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	RequestsTotal     uint64 `json:"requests_total,omitempty"`
	Responses5xx      uint64 `json:"responses_5xx_total,omitempty"`
	RequestsDelta     uint64 `json:"requests_delta,omitempty"`
	Responses5xxDelta uint64 `json:"responses_5xx_delta,omitempty"`
}

// MonitorStatus 与 CLI 读取的状态文件同形。写的是 Monitor 自己的 pid 与最近一次采集结果，
// 不写被测进程的内部状态。
type MonitorStatus struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	PID           int    `json:"pid"`
	Healthy       bool   `json:"healthy"`
	LastCollectAt string `json:"last_collect_at,omitempty"`
	LastEventAt   string `json:"last_event_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// Sink 负责三件事：追加原始快照、追加事件（带轮转）、原子写状态文件。
type Sink struct {
	Dir           string
	EventMaxBytes int64
	EventKeep     int
}

func (s *Sink) path(name string) string { return filepath.Join(s.Dir, name) }

// AppendSnapshot 把原始快照追加成一行。快照文件与事件文件用同一套上限轮转，
// 否则每 5 秒一行会无上限增长。
func (s *Sink) AppendSnapshot(raw json.RawMessage) error {
	max, keep := s.limits()
	return appendLine(s.path(SnapshotsFile), raw, max, keep)
}

// AppendEvent 追加一条事件。
func (s *Sink) AppendEvent(ev Event) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	max, keep := s.limits()
	return appendLine(s.path(EventsFile), line, max, keep)
}

// WriteStatus 原子写状态文件：先写临时文件再改名，读侧不会读到半个 JSON。
func (s *Sink) WriteStatus(st MonitorStatus) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", s.Dir, err)
	}
	st.SchemaVersion = StatusSchemaVersion
	raw, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	final := s.path(StatusFile)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}

func (s *Sink) limits() (int64, int) {
	max := s.EventMaxBytes
	if max <= 0 {
		max = DefaultEventMaxBytes
	}
	keep := s.EventKeep
	if keep < 1 {
		keep = DefaultEventKeep
	}
	return max, keep
}

// appendLine 追加一行 JSONL。max>0 时先看大小，超过上限就把旧文件轮转出去。
func appendLine(path string, line []byte, max int64, keep int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if max > 0 {
		if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line))+1 > max {
			if err := rotate(path, keep); err != nil {
				return err
			}
		}
	}
	buf := make([]byte, 0, len(line)+1)
	buf = append(buf, line...)
	buf = append(buf, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		return fmt.Errorf("append %s: %w", path, err)
	}
	return f.Close()
}

// rotate 把当前文件改成 .1，原来的 .1 变 .2，超出 keep 的最老一份删除。
func rotate(path string, keep int) error {
	if keep < 1 {
		return nil
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, keep))
	for i := keep - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("rotate %s: %w", from, err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate %s: %w", path, err)
	}
	return nil
}
