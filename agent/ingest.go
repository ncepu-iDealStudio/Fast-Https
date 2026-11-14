package main

import (
	"strings"
	"time"
)

// IngestResult 是一次尾读的计数。
type IngestResult struct {
	Added   int
	Skipped int
}

// IngestOnce 从上次偏移继续读 events.jsonl。同一 event_id 不新建任务。
// 坏行记入审计并跳过。文件不存在时不做任何事。
func IngestOnce(monitorDir, stateDir string, now time.Time) (IngestResult, error) {
	st, err := OpenStore(stateDir)
	if err != nil {
		return IngestResult{}, err
	}
	defer st.Close()
	return ingestStore(st, monitorDir, now)
}

func ingestStore(st *Store, monitorDir string, now time.Time) (IngestResult, error) {
	offset, prefix, err := st.Cursor()
	if err != nil {
		return IngestResult{}, err
	}
	next, prefix, lines, err := readNewLines(monitorDir, offset, prefix)
	if err != nil {
		return IngestResult{}, err
	}
	var res IngestResult
	for _, line := range lines {
		ev, perr := parseLine(strings.TrimSpace(line))
		if perr != nil || (ev.Type == "" && ev.Message == "" && ev.Time == "") {
			if err := st.Audit("bad_line", perrString(perr), now); err != nil {
				return res, err
			}
			res.Skipped++
			continue
		}
		id := eventID(ev)
		added, err := st.InsertEvent(ev, id, now)
		if err != nil {
			return res, err
		}
		if added {
			res.Added++
		}
	}
	if err := st.SetCursor(next, prefix); err != nil {
		return res, err
	}
	return res, nil
}

func perrString(err error) string {
	if err == nil {
		return "empty event"
	}
	return err.Error()
}
