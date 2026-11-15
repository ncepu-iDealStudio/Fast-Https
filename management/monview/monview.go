// Package monview 只读 Monitor 写下的状态文件和事件文件。
//
// CLI 不连接 Monitor 进程，也不负责启停。文件不存在表示尚未安装。
// 事件行是不可信数据，只解析白名单字段并脱敏后返回。
//
// 状态文件 status.json（schema_version "1"）：
//
//	{"schema_version":"1","status":"running","version":"...","pid":1,
//	 "healthy":true,"last_collect_at":"...","last_event_at":"...","last_error":"..."}
//
// 事件文件 events.jsonl，每行一个对象，只取 time、type、severity、message。
// Monitor 进程本体按 Monitor 计划第 6、7 步另写；本包只约定它落盘的形状。
package monview

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"fast-https/infra/logging"
	"fast-https/management/logquery"
)

const (
	SchemaVersion = "1"
	EnvDir        = "FASTHTTPS_MONITOR_DIR"
	StatusFile    = "status.json"
	EventsFile    = "events.jsonl"

	StatusNotInstalled = "not_installed"
	StatusRunning      = "running"
	StatusStale        = "stale"
	StatusUnhealthy    = "unhealthy"
	StatusUnknown      = "unknown"

	DefaultStaleAfter = 30 * time.Second
	DefaultLimit      = 100
	MaxLimit          = 1000
	maxScanBytes      = 1 << 20
	maxFieldBytes     = 1024
)

var (
	ErrSymlink = errors.New("monitor file is a symlink and will not be followed")
	ErrSchema  = errors.New("monitor status schema is not supported")
)

// FilterError 是 --since / --severity / --limit 不合法。
type FilterError struct{ Msg string }

func (e *FilterError) Error() string { return e.Msg }

// Status 是 monitor status 的数据。
type Status struct {
	Status        string `json:"status"`
	ObservedAt    string `json:"observed_at"`
	Source        string `json:"source"`
	Version       string `json:"version,omitempty"`
	PID           *int   `json:"pid,omitempty"`
	Healthy       *bool  `json:"healthy,omitempty"`
	LastCollectAt string `json:"last_collect_at,omitempty"`
	LastEventAt   string `json:"last_event_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// StatusInput 指定要读的目录。Dir 为空时用环境变量，再没有则用 logRoot/monitor。
type StatusInput struct {
	Dir        string
	LogRoot    string
	Now        time.Time
	StaleAfter time.Duration
}

// Event 是返回给调用方的一条事件。只含白名单字段。
type Event struct {
	Time     string `json:"time"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// EventsResult 是一次事件查询。
type EventsResult struct {
	Events    []Event `json:"events"`
	Skipped   int     `json:"skipped"`
	Truncated bool    `json:"truncated"`
}

// EventsInput 是事件查询条件。
type EventsInput struct {
	Dir      string
	LogRoot  string
	Since    string
	Limit    int
	Severity string
	Now      time.Time
}

// DefaultDir 是日志目录下的 monitor 子目录。
func DefaultDir(logRoot string) string {
	return filepath.Join(logRoot, "monitor")
}

// ResolveDir 按 显式目录 > FASTHTTPS_MONITOR_DIR > 日志目录/monitor 取值。
func ResolveDir(dir, logRoot string) string {
	if v := strings.TrimSpace(dir); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(EnvDir)); v != "" {
		return v
	}
	if strings.TrimSpace(logRoot) == "" {
		logRoot = "."
	}
	return DefaultDir(logRoot)
}

// ReadStatus 读取状态文件。文件不存在时 status 为 not_installed，不返回错误。
func ReadStatus(in StatusInput) (Status, []string, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	staleAfter := in.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	dir := ResolveDir(in.Dir, in.LogRoot)
	out := Status{
		ObservedAt: now.Format(time.RFC3339),
		Source:     "monitor_status_file",
		Status:     StatusNotInstalled,
	}
	path := filepath.Join(dir, StatusFile)
	raw, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil, nil
		}
		if errors.Is(err, ErrSymlink) {
			return Status{}, nil, err
		}
		return Status{}, nil, err
	}
	var file statusFile
	if err := json.Unmarshal(raw, &file); err != nil {
		out.Status = StatusUnknown
		return out, []string{"monitor status file is not valid JSON"}, nil
	}
	if file.SchemaVersion != "" && file.SchemaVersion != SchemaVersion {
		return Status{}, nil, fmt.Errorf("%w: got %q", ErrSchema, file.SchemaVersion)
	}
	out.Version = file.Version
	if file.PID > 0 {
		pid := file.PID
		out.PID = &pid
	}
	out.Healthy = file.Healthy
	out.LastCollectAt = file.LastCollectAt
	out.LastEventAt = file.LastEventAt
	out.LastError = logquery.Redact(file.LastError)
	out.Status = StatusRunning
	if file.Status != "" {
		out.Status = file.Status
	}
	var warnings []string
	if at, ok := parseTime(file.LastCollectAt); ok && now.Sub(at) > staleAfter {
		out.Status = StatusStale
		warnings = append(warnings, "monitor status file is older than "+staleAfter.String())
	} else if file.LastCollectAt == "" {
		out.Status = StatusStale
		warnings = append(warnings, "monitor status file has no last_collect_at")
	}
	if out.Status != StatusStale && file.Healthy != nil && !*file.Healthy {
		out.Status = StatusUnhealthy
	}
	return out, warnings, nil
}

// ReadEvents 从文件尾部读取事件。坏行跳过并计数，不执行其中的文本。
func ReadEvents(in EventsInput) (EventsResult, []string, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := in.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 0 {
		return EventsResult{}, nil, &FilterError{Msg: "--limit must not be negative"}
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity != "" && !allowedSeverity(severity) {
		return EventsResult{}, nil, &FilterError{Msg: "invalid --severity " + in.Severity + " (want info, warning, error, or critical)"}
	}
	var since time.Time
	var hasSince bool
	if strings.TrimSpace(in.Since) != "" {
		t, err := parseSince(in.Since, now)
		if err != nil {
			return EventsResult{}, nil, err
		}
		since, hasSince = t, true
	}

	dir := ResolveDir(in.Dir, in.LogRoot)
	path := filepath.Join(dir, EventsFile)
	raw, truncated, err := readTail(path, maxScanBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return EventsResult{Events: []Event{}}, []string{"monitor event file is not present"}, nil
		}
		return EventsResult{}, nil, err
	}

	lines := splitLines(raw)
	if truncated && len(lines) > 0 {
		lines = lines[1:]
	}
	var matched []Event
	skipped := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		ev, ok := parseEvent(line)
		if !ok {
			skipped++
			continue
		}
		if severity != "" && !strings.EqualFold(ev.Severity, severity) {
			continue
		}
		if hasSince {
			at, ok := parseTime(ev.Time)
			if !ok || at.Before(since) {
				continue
			}
		}
		matched = append(matched, ev)
		if len(matched) >= limit {
			break
		}
	}
	res := EventsResult{Events: matched, Skipped: skipped, Truncated: truncated}
	if res.Events == nil {
		res.Events = []Event{}
	}
	if len(matched) >= limit {
		res.Truncated = true
	}
	var warnings []string
	if skipped > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped %d non-JSON event line(s)", skipped))
	}
	return res, warnings, nil
}

type statusFile struct {
	SchemaVersion string `json:"schema_version"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	PID           int    `json:"pid"`
	Healthy       *bool  `json:"healthy"`
	LastCollectAt string `json:"last_collect_at"`
	LastEventAt   string `json:"last_event_at"`
	LastError     string `json:"last_error"`
}

type rawEvent struct {
	Time     string `json:"time"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func parseEvent(line string) (Event, bool) {
	var raw rawEvent
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return Event{}, false
	}
	if raw.Time == "" && raw.Type == "" && raw.Message == "" {
		return Event{}, false
	}
	msg := logquery.Redact(raw.Message)
	return Event{
		Time:     raw.Time,
		Type:     raw.Type,
		Severity: strings.ToLower(raw.Severity),
		Message:  clip(msg, maxFieldBytes),
	}, true
}

func allowedSeverity(s string) bool {
	switch s {
	case "info", "warning", "error", "critical":
		return true
	default:
		return false
	}
}

func parseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return time.Time{}, &FilterError{Msg: "duration must not be negative"}
		}
		return now.Add(-d), nil
	}
	if t, ok := parseTime(s); ok {
		return t, nil
	}
	return time.Time{}, &FilterError{Msg: "invalid --since " + s + " (want 1h, 30m, or RFC3339)"}
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{logging.TimeFormat, time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func readFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	return os.ReadFile(path)
}

func readTail(path string, max int) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	size := info.Size()
	if size <= int64(max) {
		raw, err := io.ReadAll(f)
		return raw, false, err
	}
	if _, err := f.Seek(size-int64(max), io.SeekStart); err != nil {
		return nil, false, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(max)))
	return raw, true, err
}

func splitLines(raw []byte) []string {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), maxScanBytes)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for !utf8.ValidString(s) && s != "" {
		s = s[:len(s)-1]
	}
	return s
}
