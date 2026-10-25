package appfirewall

import (
	"testing"

	"fast-https/runtime/request"
)

func newSQLTestReq(query, body string) *request.Request {
	req := request.RequestInit(false)
	if query != "" {
		req.Query["q"] = query
	}
	req.Body.WriteString(body)
	return req
}

// 高置信度注入特征必须被拒绝
func TestHandleSqlRejectsInjection(t *testing.T) {
	payloads := []string{
		"' UNION SELECT * FROM users--",
		"; DROP TABLE users;--",
		"' OR '1'='1",
		"1 OR 1=1",
		"admin' AND 1=1--",
		"sleep(5)--",
		"SELECT * FROM information_schema.tables",
		"1; DELETE FROM orders",
	}
	for _, p := range payloads {
		if HandleSql(newSQLTestReq(p, "")) {
			t.Errorf("payload %q should be rejected", p)
		}
		if HandleSql(newSQLTestReq("", p)) {
			t.Errorf("body payload %q should be rejected", p)
		}
	}
}

// 正常内容不能误杀：日期、编号、常规文案
func TestHandleSqlAllowsNormalContent(t *testing.T) {
	safe := []string{
		"2026-10-25",
		"hello world",
		"user-12345",
		"a-b-c",
		"价格 100 元",
		"search=golang",
	}
	for _, p := range safe {
		if !HandleSql(newSQLTestReq(p, "")) {
			t.Errorf("normal content %q should not be rejected", p)
		}
		if !HandleSql(newSQLTestReq("", p)) {
			t.Errorf("normal body %q should not be rejected", p)
		}
	}
}

// sql 规则现在具备拦截能力
func TestSqlRuleBlocks(t *testing.T) {
	if !Blocks("sql") {
		t.Error("sql rule should be a blocking rule now")
	}
}
