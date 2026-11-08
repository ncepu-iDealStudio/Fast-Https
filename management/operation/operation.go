// Package operation 持久化管理操作：状态机、幂等索引、崩溃对账。
package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fast-https/management/state"
)

const (
	StatusQueued           = "queued"
	StatusRunning          = "running"
	StatusSucceeded        = "succeeded"
	StatusFailed           = "failed"
	StatusCancelled        = "cancelled"
	StatusUnknown          = "unknown"
	StatusRecoveryRequired = "recovery_required"

	DefaultTTL   = 7 * 24 * time.Hour
	DefaultLimit = 50
	MaxLimit     = 1000
)

var (
	ErrNotFound          = errors.New("operation not found")
	ErrRequestIDConflict = errors.New("request id conflict")
)

// Record 是一条落盘的 operation。
type Record struct {
	ID            string         `json:"operation_id"`
	RequestID     string         `json:"request_id,omitempty"`
	Command       string         `json:"command"`
	Status        string         `json:"status"`
	CreatedAt     string         `json:"created_at"`
	StartedAt     string         `json:"started_at,omitempty"`
	FinishedAt    string         `json:"finished_at,omitempty"`
	OwnerPID      int            `json:"owner_pid,omitempty"`
	OwnerStartKey string         `json:"owner_start_key,omitempty"`
	Phase         string         `json:"phase,omitempty"`
	ArgsHash      string         `json:"args_hash,omitempty"`
	Result        map[string]any `json:"result,omitempty"`
	Error         *ErrorBody     `json:"error"`
}

// ErrorBody 是失败时的简短原因，不含秘密。
type ErrorBody struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type requestEntry struct {
	ArgsHash    string `json:"args_hash"`
	OperationID string `json:"operation_id"`
	CreatedAt   string `json:"created_at"`
}

// Store 读写 operations/ 与 requests.json。
type Store struct {
	Dir state.Dir
	TTL time.Duration
}

func (s Store) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return DefaultTTL
}

func (s Store) now() time.Time { return s.Dir.CurrentTime() }

// HashArgs 是命令名加规范化参数的 sha256。不含 request-id，调用方不要传入秘密原文。
func HashArgs(command string, args map[string]string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "request-id" || lk == "request_id" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(command)
	for _, k := range keys {
		fmt.Fprintf(&b, "\n%s=%s", k, args[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// BeginInput 描述一次要登记的操作。
type BeginInput struct {
	RequestID string
	Command   string
	Args      map[string]string
	Phase     string
}

// Begin 登记一条 queued/running 记录。同一 request_id 同参数返回已有记录。
func (s Store) Begin(in BeginInput) (Record, error) {
	release, err := s.Dir.Lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	if err := s.Dir.Ensure(); err != nil {
		return Record{}, err
	}

	hash := HashArgs(in.Command, in.Args)
	now := s.now()
	reqs, err := s.loadRequests()
	if err != nil {
		return Record{}, err
	}
	s.expireRequests(reqs, now)
	if id := strings.TrimSpace(in.RequestID); id != "" {
		if e, ok := reqs[id]; ok {
			if e.ArgsHash != hash {
				return Record{}, ErrRequestIDConflict
			}
			return s.read(e.OperationID)
		}
	}

	rec := Record{
		ID:            s.nextID(now),
		RequestID:     strings.TrimSpace(in.RequestID),
		Command:       in.Command,
		Status:        StatusRunning,
		CreatedAt:     now.UTC().Format(time.RFC3339Nano),
		StartedAt:     now.UTC().Format(time.RFC3339Nano),
		OwnerPID:      s.Dir.CurrentPID(),
		OwnerStartKey: state.StartKey(s.Dir.CurrentPID()),
		Phase:         in.Phase,
		ArgsHash:      hash,
		Result:        map[string]any{},
	}
	if rec.Phase == "" {
		rec.Phase = "started"
	}
	if err := s.write(rec); err != nil {
		return Record{}, err
	}
	if rec.RequestID != "" {
		reqs[rec.RequestID] = requestEntry{ArgsHash: hash, OperationID: rec.ID, CreatedAt: rec.CreatedAt}
		if err := s.saveRequests(reqs); err != nil {
			return Record{}, err
		}
	}
	return rec, nil
}

// Get 读取一条记录，必要时把失联的 running 对账成 unknown。
func (s Store) Get(id string) (Record, error) {
	if _, err := os.Stat(s.Dir.OperationsDir()); os.IsNotExist(err) {
		return Record{}, ErrNotFound
	}
	release, err := s.Dir.Lock()
	if err != nil {
		return Record{}, err
	}
	defer release()
	rec, err := s.read(id)
	if err != nil {
		return Record{}, err
	}
	if rec.Status == StatusRunning && !s.Dir.IsAlive(rec.OwnerPID, rec.OwnerStartKey) {
		rec.Status = StatusUnknown
		if rec.Result == nil {
			rec.Result = map[string]any{}
		}
		rec.Result["last_phase"] = rec.Phase
		rec.FinishedAt = s.now().UTC().Format(time.RFC3339Nano)
		if err := s.write(rec); err != nil {
			return Record{}, err
		}
	}
	return rec, nil
}

// ListFilter 是 operation list 的白名单过滤。
type ListFilter struct {
	Status string
	Since  time.Time
	Limit  int
}

// List 列出记录，新的在前，并对账失联的 running。
func (s Store) List(f ListFilter) ([]Record, error) {
	if _, err := os.Stat(s.Dir.OperationsDir()); os.IsNotExist(err) {
		return []Record{}, nil
	}
	release, err := s.Dir.Lock()
	if err != nil {
		return nil, err
	}
	defer release()
	entries, err := os.ReadDir(s.Dir.OperationsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	var out []Record
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() > entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "op_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		rec, err := s.read(id)
		if err != nil {
			continue
		}
		if rec.Status == StatusRunning && !s.Dir.IsAlive(rec.OwnerPID, rec.OwnerStartKey) {
			rec.Status = StatusUnknown
			if rec.Result == nil {
				rec.Result = map[string]any{}
			}
			rec.Result["last_phase"] = rec.Phase
			rec.FinishedAt = s.now().UTC().Format(time.RFC3339Nano)
			_ = s.write(rec)
		}
		if f.Status != "" && rec.Status != f.Status {
			continue
		}
		if !f.Since.IsZero() {
			created, err := time.Parse(time.RFC3339Nano, rec.CreatedAt)
			if err != nil {
				created, err = time.Parse(time.RFC3339, rec.CreatedAt)
			}
			if err == nil && created.Before(f.Since) {
				continue
			}
		}
		out = append(out, rec)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s Store) pathOf(id string) string {
	return filepath.Join(s.Dir.OperationsDir(), id+".json")
}

func (s Store) read(id string) (Record, error) {
	var rec Record
	if err := readOp(s.pathOf(id), &rec); err != nil {
		if os.IsNotExist(err) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	return rec, nil
}

func (s Store) write(rec Record) error {
	return writeOp(s.pathOf(rec.ID), rec)
}

func (s Store) nextID(now time.Time) string {
	day := now.UTC().Format("20060102")
	prefix := "op_" + day + "_"
	n := 1
	entries, _ := os.ReadDir(s.Dir.OperationsDir())
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		var k int
		if _, err := fmt.Sscanf(name, prefix+"%d", &k); err == nil && k >= n {
			n = k + 1
		}
	}
	return fmt.Sprintf("%s%03d", prefix, n)
}

func (s Store) loadRequests() (map[string]requestEntry, error) {
	var m map[string]requestEntry
	err := readOp(s.Dir.RequestsPath(), &m)
	if os.IsNotExist(err) {
		return map[string]requestEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]requestEntry{}
	}
	return m, nil
}

func (s Store) saveRequests(m map[string]requestEntry) error {
	return writeOp(s.Dir.RequestsPath(), m)
}

func (s Store) expireRequests(m map[string]requestEntry, now time.Time) {
	cut := now.Add(-s.ttl())
	for id, e := range m {
		t, err := time.Parse(time.RFC3339Nano, e.CreatedAt)
		if err != nil {
			t, err = time.Parse(time.RFC3339, e.CreatedAt)
		}
		if err != nil || t.Before(cut) {
			delete(m, id)
		}
	}
}

func readOp(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func writeOp(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return state.WriteFile(path, data, 0o600)
}
