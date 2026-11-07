package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeQueryLog(t *testing.T, name string, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name+".log"),
		[]byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sampleAccessLines() []string {
	return []string{
		`{"level":"info","kind":"access","time":"2026-11-07T10:00:01+08:00","method":"GET","path":"/a","status":200,"size":10,"host":"localhost"}`,
		`{"level":"warn","kind":"access","time":"2026-11-07T10:00:02+08:00","method":"POST","path":"/b","status":404,"size":20,"host":"localhost"}`,
		`{"level":"error","kind":"access","time":"2026-11-07T10:00:03+08:00","method":"GET","path":"/c","status":500,"size":30,"host":"example.com"}`,
		`{"level":"info","kind":"access","time":"2026-11-07T10:00:04+08:00","method":"GET","path":"/d","status":200,"size":40,"host":"localhost"}`,
		`{"level":"warn","kind":"access","time":"2026-11-07T10:00:05+08:00","method":"GET","path":"/e","status":403,"size":50,"host":"example.com"}`,
	}
}

func TestQueryLogsReverseOrderAndPaging(t *testing.T) {
	dir := writeQueryLog(t, "access", sampleAccessLines())

	p1, err := queryLogs(dir, LogQuery{Name: "access", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Lines) != 2 || p1.Lines[0].Path != "/e" || p1.Lines[1].Path != "/d" {
		t.Fatalf("第一页应从最新往前：%+v", p1.Lines)
	}
	if !p1.HasMore {
		t.Fatal("还有更早的记录，HasMore 应为 true")
	}

	p2, err := queryLogs(dir, LogQuery{Name: "access", Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Lines) != 2 || p2.Lines[0].Path != "/c" || p2.Lines[1].Path != "/b" {
		t.Fatalf("第二页偏移不对：%+v", p2.Lines)
	}

	p3, err := queryLogs(dir, LogQuery{Name: "access", Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(p3.Lines) != 1 || p3.Lines[0].Path != "/a" || p3.HasMore {
		t.Fatalf("最后一页不对：%+v hasMore=%v", p3.Lines, p3.HasMore)
	}
}

func TestQueryLogsFilterByLevelStatusAndSearch(t *testing.T) {
	dir := writeQueryLog(t, "access", sampleAccessLines())

	warn, err := queryLogs(dir, LogQuery{Name: "access", Level: "warn", Limit: 10})
	if err != nil || len(warn.Lines) != 2 {
		t.Fatalf("level=warn 应命中 2 条：%+v err=%v", warn.Lines, err)
	}
	four, err := queryLogs(dir, LogQuery{Name: "access", Status: "4", Limit: 10})
	if err != nil || len(four.Lines) != 2 {
		t.Fatalf("status=4xx 应命中 2 条：%+v err=%v", four.Lines, err)
	}
	search, err := queryLogs(dir, LogQuery{Name: "access", Search: "example.com", Limit: 10})
	if err != nil || len(search.Lines) != 2 {
		t.Fatalf("关键词应命中 2 条：%+v err=%v", search.Lines, err)
	}
	combo, err := queryLogs(dir, LogQuery{Name: "access", Level: "warn", Status: "4", Limit: 10})
	if err != nil || len(combo.Lines) != 2 {
		t.Fatalf("组合筛选：%+v err=%v", combo.Lines, err)
	}
}

func TestQueryLogsTimeRange(t *testing.T) {
	dir := writeQueryLog(t, "access", sampleAccessLines())
	got, err := queryLogs(dir, LogQuery{
		Name: "access", Since: "2026-11-07T10:00:02", Until: "2026-11-07T10:00:04", Limit: 10,
	})
	if err != nil || len(got.Lines) != 3 {
		t.Fatalf("时间范围应命中 3 条：%+v err=%v", got.Lines, err)
	}
}

func TestQueryLogsAcrossChunksAndNonJSON(t *testing.T) {
	lines := make([]string, 0, 1500)
	for i := 0; i < 1500; i++ {
		lines = append(lines, fmt.Sprintf(
			`{"level":"info","kind":"system","time":"2026-11-07T10:00:00+08:00","message":"line %d padding padding padding"}`, i))
	}
	lines = append(lines, "plain text line, not json")
	dir := writeQueryLog(t, "system", lines)

	got, err := queryLogs(dir, LogQuery{Name: "system", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasMore || len(got.Lines) != 3 {
		t.Fatalf("跨块读取失败：%+v hasMore=%v", got.Lines, got.HasMore)
	}
	if got.Lines[0].Message != "plain text line, not json" {
		t.Fatalf("最新一行应是纯文本，实际 %q", got.Lines[0].Message)
	}
	if !strings.Contains(got.Lines[1].Message, "line 1499") {
		t.Fatalf("倒序第二条应是 line 1499：%q", got.Lines[1].Message)
	}
	// 有结构化筛选时，非 JSON 行不应混进来
	filtered, err := queryLogs(dir, LogQuery{Name: "system", Level: "info", Limit: 3})
	if err != nil || len(filtered.Lines) != 3 || filtered.Lines[0].Message == "plain text line, not json" {
		t.Fatalf("level 筛选应排除非 JSON 行：%+v err=%v", filtered.Lines, err)
	}
}

func TestQueryLogsRejectsUnknownName(t *testing.T) {
	if _, err := queryLogs(t.TempDir(), LogQuery{Name: "passwd"}); err == nil {
		t.Fatal("未知日志类型必须拒绝")
	}
}
