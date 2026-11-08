// Package logquery 只读四路固定日志文件。
//
// 过滤条件是白名单字段，不是表达式。日志行一律当数据：解析、脱敏、截断后放进结构体，
// 不拼进路径或命令，也不执行其中的任何文本。
package logquery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fast-https/config"
	"fast-https/infra/jsonc"
	"fast-https/infra/logging"
)

const (
	// MaxScanBytes 是一次查询从文件尾往前最多读取的字节数。
	// 到上限就停，并标 scan_truncated，避免把一份很大的 access.log 整文件读进内存。
	MaxScanBytes = 64 << 20
	// MaxOutputBytes 是返回记录序列化后的总上限。
	MaxOutputBytes = 256 << 10
	// MaxFieldBytes 是单条 message / path 写回前的最长字节数。
	MaxFieldBytes = 1024
	// DefaultLimit 与 MaxLimit 是 --limit 的默认值和上限。
	DefaultLimit = 100
	MaxLimit     = 1000

	chunkSize    = 64 << 10
	maxLineBytes = 1 << 20
)

// 文件名由代码固定。不接受调用方传入的路径片段。
var kindFiles = map[string]string{
	logging.KindAccess: logging.AccessLogName,
	logging.KindError:  logging.ErrorLogName,
	logging.KindSystem: logging.SystemLogName,
	logging.KindSafe:   logging.SafeLogName,
}

// SourceError 表示日志文件不能作为数据源：不存在，或者是符号链接。
// 符号链接直接拒绝，避免 --kind 被带到日志目录之外。
type SourceError struct {
	File string
	Kind string
}

func (e *SourceError) Error() string {
	switch e.Kind {
	case "symlink":
		return "log file is a symlink and will not be followed: " + e.File
	default:
		return "log file not found: " + e.File
	}
}

// Query 是一次查询。Kind 决定读哪个固定文件；其余字段都是可选过滤。
// maxScan / maxOutput / maxField 只给测试收紧上限，零值用上面的常量。
type Query struct {
	Dir        string
	Kind       string
	Since      string
	Until      string
	Limit      int
	Domain     string
	Status     string
	Level      string
	Method     string
	PathPrefix string
	Reason     string
	Now        time.Time

	maxScan   int64
	maxOutput int
	maxField  int
}

// Record 只含白名单字段。没有原始行，也没有 user_agent。
type Record struct {
	Time       string `json:"time,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Level      string `json:"level,omitempty"`
	Message    string `json:"message,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     *int   `json:"status,omitempty"`
	Size       *int   `json:"size,omitempty"`
	Host       string `json:"host,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// Result 是一次查询的数据和它的覆盖范围。
type Result struct {
	Records         []Record `json:"records"`
	Source          Source   `json:"source"`
	TimeRange       Range    `json:"time_range"`
	RangeComplete   bool     `json:"range_complete"`
	ScanTruncated   bool     `json:"scan_truncated"`
	OutputTruncated bool     `json:"output_truncated"`
	Truncated       bool     `json:"truncated"`
	Limit           int      `json:"limit"`
}

// Source 标明读了哪个固定文件、实际读了多少字节。
type Source struct {
	File         string `json:"file"`
	ScannedBytes int64  `json:"scanned_bytes"`
}

// Range 是请求的起点和这次扫描里见到的最早时间。两者都是日志时间格式，可直接比较。
type Range struct {
	Since           string `json:"since,omitempty"`
	EarliestScanned string `json:"earliest_scanned,omitempty"`
}

// ResolveLogDir 从主配置读 log_root。读不到或没写时用默认日志目录。
// 配置有问题只产生告警：日志查询不因为主配置暂时损坏而完全不可用。
func ResolveLogDir(configPath string) (string, []string) {
	root := config.DEFAULT_LOG_ROOT
	var warnings []string
	if strings.TrimSpace(configPath) != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("config %s unreadable (%v); using default log directory", configPath, err))
		} else {
			var raw map[string]any
			if err := jsonc.Unmarshal(data, &raw); err != nil {
				warnings = append(warnings, fmt.Sprintf("config %s is not valid JSON; using default log directory", configPath))
			} else if v, ok := raw["log_root"].(string); ok && strings.TrimSpace(v) != "" {
				root = strings.TrimSpace(v)
			}
		}
	}
	if !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
	}
	return root, warnings
}

// Run 执行一次查询。过滤条件不合法时不打开文件。
func Run(q Query) (Result, []string, error) {
	fileName, ok := kindFiles[q.Kind]
	if !ok {
		return Result{}, nil, badFilter("invalid --kind %q (want access, error, system, or safe)", q.Kind)
	}
	now := q.Now
	if now.IsZero() {
		now = time.Now()
	}
	since, err := parseBound(q.Since, now)
	if err != nil {
		return Result{}, nil, err
	}
	until, err := parseBound(q.Until, now)
	if err != nil {
		return Result{}, nil, err
	}
	status, err := parseStatus(q.Status)
	if err != nil {
		return Result{}, nil, err
	}
	limit, limitWarning, err := normalizeLimit(q.Limit)
	if err != nil {
		return Result{}, nil, err
	}

	var warnings []string
	if limitWarning != "" {
		warnings = append(warnings, limitWarning)
	}

	path := filepath.Join(q.Dir, fileName)
	f, size, err := openLog(path, fileName)
	if err != nil {
		return Result{}, nil, err
	}
	defer f.Close()

	maxScan := q.maxScan
	if maxScan <= 0 {
		maxScan = MaxScanBytes
	}
	maxOut := q.maxOutput
	if maxOut <= 0 {
		maxOut = MaxOutputBytes
	}
	maxField := q.maxField
	if maxField <= 0 {
		maxField = MaxFieldBytes
	}

	res := Result{
		Records:   []Record{},
		Source:    Source{File: fileName},
		TimeRange: Range{Since: since},
		Limit:     limit,
	}

	var (
		outputBytes int
		missingHost bool
		missingTime bool
		nonJSON     int
		fieldCut    bool
	)

	scanned, hitStart, scanErr := scanReverse(f, size, maxScan, func(line string) {
		obj, ok := parseObject(line)
		if !ok {
			nonJSON++
			return
		}
		rec, hasHost, hasTime := project(obj)
		if hasTime && (res.TimeRange.EarliestScanned == "" || rec.Time < res.TimeRange.EarliestScanned) {
			res.TimeRange.EarliestScanned = rec.Time
		}
		if q.Domain != "" && !hasHost {
			missingHost = true
		}
		if (since != "" || until != "") && !hasTime {
			missingTime = true
		}
		if !accept(q, status, since, until, obj, rec, hasHost, hasTime) {
			return
		}
		// 过滤用原文。写出前再脱敏并截断，避免把口令截成半截后才遮蔽。
		var cut bool
		rec.Path = clip(redactText(rec.Path), maxField, &cut)
		rec.Message = clip(redactText(rec.Message), maxField, &cut)
		rec.Reason = redactText(rec.Reason)
		rec.Truncated = cut
		if cut {
			fieldCut = true
		}
		if len(res.Records) >= limit {
			return
		}
		encoded, err := json.Marshal(rec)
		if err != nil {
			return
		}
		if outputBytes+len(encoded) > maxOut && len(res.Records) > 0 {
			res.OutputTruncated = true
			return
		}
		outputBytes += len(encoded)
		res.Records = append(res.Records, rec)
		if outputBytes > maxOut {
			res.OutputTruncated = true
		}
	})
	if scanErr != nil {
		return Result{}, nil, scanErr
	}

	res.Source.ScannedBytes = scanned
	res.ScanTruncated = !hitStart
	res.Truncated = res.ScanTruncated || res.OutputTruncated || fieldCut
	res.RangeComplete = rangeComplete(hitStart, since, res.TimeRange.EarliestScanned)

	if missingHost {
		warnings = append(warnings, "some scanned records have no host field; --domain does not match them")
	}
	if missingTime {
		warnings = append(warnings, "some scanned records have no time field; --since/--until does not match them")
	}
	if nonJSON > 0 {
		warnings = append(warnings, fmt.Sprintf("skipped %d non-json log lines", nonJSON))
	}
	return res, warnings, nil
}

// rangeComplete 表示这次读到的内容盖住了请求的时间范围。
// 扫描被字节上限截断时，只要已经看到不晚于 since 的记录，范围就是完整的；
// 没看到，就说明更早的数据还在窗口外面（或已经轮转走了）。
func rangeComplete(hitStart bool, since, earliest string) bool {
	if since != "" && earliest != "" && earliest <= since {
		return true
	}
	if !hitStart {
		return false
	}
	if since != "" && earliest != "" && earliest > since {
		return false
	}
	return true
}

func normalizeLimit(n int) (int, string, error) {
	if n == 0 {
		return DefaultLimit, "", nil
	}
	if n < 0 {
		return 0, "", badFilter("--limit must be greater than 0")
	}
	if n > MaxLimit {
		return MaxLimit, fmt.Sprintf("limit capped at %d", MaxLimit), nil
	}
	return n, "", nil
}

func accept(q Query, status statusFilter, since, until string, obj map[string]any, rec Record, hasHost, hasTime bool) bool {
	if !matchTime(since, until, rec.Time, hasTime) {
		return false
	}
	if q.Domain != "" {
		if !hasHost || !strings.EqualFold(rec.Host, q.Domain) {
			return false
		}
	}
	code, hasStatus := asInt(obj[logging.FieldStatus])
	if !status.match(code, hasStatus) {
		return false
	}
	if !matchExact(q.Level, rec.Level) || !matchExact(q.Method, rec.Method) || !matchExact(q.Reason, rawString(obj, logging.FieldReason)) {
		return false
	}
	if !matchPrefix(q.PathPrefix, rawString(obj, logging.FieldPath)) {
		return false
	}
	return true
}

func project(obj map[string]any) (Record, bool, bool) {
	var rec Record
	rec.Time = rawString(obj, "time")
	rec.Kind = rawString(obj, logging.FieldKind)
	rec.Level = rawString(obj, "level")
	rec.Method = rawString(obj, logging.FieldMethod)
	rec.Host = rawString(obj, logging.FieldHost)
	rec.RemoteAddr = rawString(obj, logging.FieldRemoteAddr)
	rec.Reason = rawString(obj, logging.FieldReason)
	rec.Message = rawString(obj, "message")
	rec.Path = rawString(obj, logging.FieldPath)
	if n, ok := asInt(obj[logging.FieldStatus]); ok {
		rec.Status = &n
	}
	if n, ok := asInt(obj[logging.FieldSize]); ok {
		rec.Size = &n
	}
	_, hasHost := obj[logging.FieldHost]
	_, hasTime := obj["time"]
	return rec, hasHost, hasTime && rec.Time != ""
}

func clip(s string, max int, cut *bool) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	*cut = true
	end := max
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	if end == 0 {
		return ""
	}
	return s[:end]
}

func rawString(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil || i > int64(^uint(0)>>1) {
			return 0, false
		}
		return int(i), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func parseObject(line string) (map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, false
	}
	return obj, true
}

func openLog(path, name string) (*os.File, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, &SourceError{File: name, Kind: "missing"}
		}
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, &SourceError{File: name, Kind: "symlink"}
	}
	if info.IsDir() {
		return nil, 0, &SourceError{File: name, Kind: "missing"}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// scanReverse 从文件尾往前按块读，最多 maxScan 字节。
// 窗口左边界正好切在一行中间时，丢掉那半行；读到文件头才把首行算进去。
// onLine 收到的是由新到旧的完整行。返回值 scanned 不超过 maxScan。
func scanReverse(f *os.File, size, maxScan int64, onLine func(string)) (scanned int64, hitStart bool, err error) {
	if size == 0 {
		return 0, true, nil
	}
	if maxScan > size {
		maxScan = size
	}
	var carry []byte
	pos := size
	for pos > 0 && scanned < maxScan {
		n := int64(chunkSize)
		if n > pos {
			n = pos
		}
		if n > maxScan-scanned {
			n = maxScan - scanned
		}
		pos -= n
		part := make([]byte, n)
		if _, err := f.ReadAt(part, pos); err != nil && err != io.EOF {
			return scanned, false, err
		}
		scanned += n
		data := append(part, carry...)
		parts := bytes.Split(data, []byte("\n"))
		if len(parts[0]) > maxLineBytes {
			carry = nil
		} else {
			carry = append([]byte(nil), parts[0]...)
		}
		for i := len(parts) - 1; i >= 1; i-- {
			line := strings.TrimRight(string(parts[i]), "\r")
			if line == "" {
				continue
			}
			if len(line) > maxLineBytes {
				continue
			}
			onLine(line)
		}
	}
	hitStart = pos == 0
	if hitStart {
		if line := strings.TrimRight(string(carry), "\r"); line != "" && len(line) <= maxLineBytes {
			onLine(line)
		}
	}
	return scanned, hitStart, nil
}
