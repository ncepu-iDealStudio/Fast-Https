package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LogQuery 是日志页一次查询：从最新往前翻页，按 JSON 字段筛选。
// Offset 是已经加载过的匹配行数，Limit 是本页条数。
type LogQuery struct {
	Name   string `json:"name"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
	Level  string `json:"level"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Search string `json:"search"`
	Since  string `json:"since"`
	Until  string `json:"until"`
}

// LogEntry 是把一行 JSON 日志摊平后的展示字段；Raw 始终保留原文。
type LogEntry struct {
	Raw        string `json:"raw"`
	Time       string `json:"time"`
	Level      string `json:"level"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     string `json:"status"`
	Size       string `json:"size"`
	Host       string `json:"host"`
	RemoteAddr string `json:"remoteAddr"`
}

type LogPage struct {
	Lines   []LogEntry `json:"lines"`
	HasMore bool       `json:"hasMore"`
	Matched int        `json:"matched"`
}

const (
	logChunkSize = 64 * 1024
	logMaxLines  = 50000
	logPageSize  = 30
	logPageMax   = 200
)

// QueryLogs 只读日志、不跟随写入句柄；倒序按块扫描，避免大 access.log
// 整文件进内存，同时支持「往下滚动加载更早的记录」。
func (s *DesktopService) QueryLogs(q LogQuery) (LogPage, error) {
	return queryLogs(resolvePaths(s.baseDir).Log, q)
}

func queryLogs(logDir string, q LogQuery) (LogPage, error) {
	name := q.Name
	if name == "" {
		name = "access"
	}
	switch name {
	case "access", "error", "system", "safe":
	default:
		return LogPage{}, fmt.Errorf("不支持的日志类型：%s", name)
	}
	if q.Limit <= 0 || q.Limit > logPageMax {
		q.Limit = logPageSize
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	f, err := os.Open(filepath.Join(logDir, name+".log"))
	if err != nil {
		return LogPage{}, fmt.Errorf("读不到 %s.log：%w", name, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return LogPage{}, err
	}
	lines, moreInFile := readLinesReverse(f, st.Size())

	page := LogPage{Lines: make([]LogEntry, 0, q.Limit)}
	skipped := 0
	for _, line := range lines {
		e := parseLogLine(line)
		if !q.matches(e) {
			continue
		}
		if skipped < q.Offset {
			skipped++
			continue
		}
		if len(page.Lines) >= q.Limit {
			page.HasMore = true
			break
		}
		page.Lines = append(page.Lines, e)
	}
	page.Matched = skipped + len(page.Lines)
	if moreInFile && len(page.Lines) < q.Limit {
		page.HasMore = true
	}
	return page, nil
}

// readLinesReverse 从文件末尾按块往前读，返回「由新到旧」的完整行。
// 块开头那一行可能不完整，留到下一轮与更早的块拼接。
func readLinesReverse(f *os.File, size int64) ([]string, bool) {
	lines := make([]string, 0, 512)
	var pending []byte
	pos := size
	stopped := false
	readErr := false
	for pos > 0 {
		n := int64(logChunkSize)
		if n > pos {
			n = pos
		}
		pos -= n
		part := make([]byte, n)
		if _, err := f.ReadAt(part, pos); err != nil {
			readErr = true
			stopped = true
			break
		}
		data := append(part, pending...)
		parts := bytes.Split(data, []byte("\n"))
		for i := len(parts) - 1; i >= 1; i-- {
			line := strings.TrimRight(string(parts[i]), "\r")
			if line != "" {
				lines = append(lines, line)
			}
			if len(lines) >= logMaxLines {
				stopped = true
				break
			}
		}
		pending = append([]byte(nil), parts[0]...)
		if stopped {
			break
		}
	}
	if !stopped {
		if line := strings.TrimRight(string(pending), "\r"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, readErr || pos > 0 || len(lines) >= logMaxLines
}

func fieldString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

func parseLogLine(line string) LogEntry {
	e := LogEntry{Raw: line, Message: line}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return e
	}
	e.Time = fieldString(m["time"])
	e.Level = fieldString(m["level"])
	e.Kind = fieldString(m["kind"])
	e.Message = fieldString(m["message"])
	e.Method = fieldString(m["method"])
	e.Path = fieldString(m["path"])
	e.Status = fieldString(m["status"])
	e.Size = fieldString(m["size"])
	e.Host = fieldString(m["host"])
	e.RemoteAddr = fieldString(m["remote_addr"])
	if e.Message == "" {
		if e.Method != "" || e.Path != "" {
			e.Message = strings.TrimSpace(e.Method + " " + e.Path + " → " + e.Status)
		} else {
			e.Message = line
		}
	}
	return e
}

// matches 用摊平后的字段做筛选；时间用 ISO 字符串的字典序比较，
// 用户填 "2026-11-07" 这种前缀也能直接当起点/终点用。
func (q LogQuery) matches(e LogEntry) bool {
	if q.Level != "" && !strings.EqualFold(e.Level, q.Level) {
		return false
	}
	if q.Kind != "" && !strings.EqualFold(e.Kind, q.Kind) {
		return false
	}
	if q.Status != "" && !strings.HasPrefix(e.Status, q.Status) {
		return false
	}
	if q.Search != "" && !strings.Contains(strings.ToLower(e.Raw), strings.ToLower(q.Search)) {
		return false
	}
	if (q.Since != "" || q.Until != "") && e.Time == "" {
		return false
	}
	if q.Since != "" && e.Time < q.Since {
		return false
	}
	// Until 是「含边界」：日志时间带时区/毫秒后缀，用户填到秒时
	// "2026-11-07T10:00:04+08:00" 会大于 "2026-11-07T10:00:04"，
	// 追加一个大于常见字符的后缀把它纳入范围。
	if q.Until != "" && e.Time > q.Until+"~" {
		return false
	}
	return true
}
