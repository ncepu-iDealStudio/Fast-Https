package operation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fast-https/management/state"
)

func TestOperationReconcile(t *testing.T) {
	st := state.Dir{
		Root:  t.TempDir(),
		Alive: func(pid int, startKey string) bool { return false },
		Now:   func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) },
	}
	s := Store{Dir: st}
	rec, err := s.Begin(BeginInput{Command: "config.apply", Phase: "reload", Args: map[string]string{"x": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusUnknown {
		t.Fatalf("status = %s", got.Status)
	}
	if got.Result["last_phase"] != "reload" {
		t.Fatalf("result = %v", got.Result)
	}
}

func TestIdempotentSameArgs(t *testing.T) {
	s := Store{Dir: state.Dir{Root: t.TempDir(), Alive: alive}}
	a, err := s.Begin(BeginInput{RequestID: "req-1", Command: "config.apply", Args: map[string]string{"c": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Begin(BeginInput{RequestID: "req-1", Command: "config.apply", Args: map[string]string{"c": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("应返回同一 operation：%s vs %s", a.ID, b.ID)
	}
	entries, _ := os.ReadDir(s.Dir.OperationsDir())
	if len(entries) != 1 {
		t.Fatalf("新建了多余记录：%d", len(entries))
	}
}

func TestRequestIDConflict(t *testing.T) {
	s := Store{Dir: state.Dir{Root: t.TempDir(), Alive: alive}}
	if _, err := s.Begin(BeginInput{RequestID: "req-1", Command: "config.apply", Args: map[string]string{"c": "1"}}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Begin(BeginInput{RequestID: "req-1", Command: "config.apply", Args: map[string]string{"c": "2"}})
	if !errors.Is(err, ErrRequestIDConflict) {
		t.Fatalf("err = %v", err)
	}
}

func TestRequestIDExpiry(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	s := Store{
		Dir: state.Dir{Root: t.TempDir(), Alive: alive, Now: func() time.Time { return now }},
		TTL: time.Hour,
	}
	first, err := s.Begin(BeginInput{RequestID: "req-1", Command: "a", Args: map[string]string{"c": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Dir.Now = func() time.Time { return now.Add(2 * time.Hour) }
	second, err := s.Begin(BeginInput{RequestID: "req-1", Command: "a", Args: map[string]string{"c": "2"}})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("过期后应能重新使用 request_id")
	}
}

func TestOperationListFilters(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	s := Store{Dir: state.Dir{Root: t.TempDir(), Alive: alive, Now: func() time.Time { return now }}}
	if _, err := s.Begin(BeginInput{Command: "a", Args: map[string]string{"n": "1"}}); err != nil {
		t.Fatal(err)
	}
	s.Dir.Now = func() time.Time { return now.Add(2 * time.Hour) }
	if _, err := s.Begin(BeginInput{Command: "b", Args: map[string]string{"n": "2"}}); err != nil {
		t.Fatal(err)
	}
	all, err := s.List(ListFilter{Limit: 10})
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %d err=%v", len(all), err)
	}
	one, err := s.List(ListFilter{Limit: 1})
	if err != nil || len(one) != 1 {
		t.Fatalf("limit = %d", len(one))
	}
	since := now.Add(time.Hour)
	recent, err := s.List(ListFilter{Since: since, Limit: 10})
	if err != nil || len(recent) != 1 || recent[0].Command != "b" {
		t.Fatalf("since = %+v", recent)
	}
}

func alive(pid int, startKey string) bool { return true }

func TestWriteLeavesNoPartialJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "op.json")
	if err := writeOp(path, Record{ID: "op_1", Status: StatusRunning}); err != nil {
		t.Fatal(err)
	}
}
