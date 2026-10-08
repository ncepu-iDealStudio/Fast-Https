package appfirewall

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"strconv"
	"strings"
	"testing"
)

func TestSqlRuleDoesNotIntercept(t *testing.T) {
	if Blocks("sql") {
		t.Fatal("sql is a stub and must not be treated as a blocking rule")
	}
	req := request.RequestInit(false)
	req.Method = "POST"
	body := `{"q":"1' OR '1'='1"}`
	req.Headers["Content-Type"] = "application/json"
	req.Headers["Content-Length"] = strconv.Itoa(len(body))
	req.Body.WriteString(body)

	if !HandleSql(req) {
		t.Fatal("sql stub should allow the request")
	}
	cfg := &listener.ListenCfg{AppFireWall: []string{"sql"}}
	if !HandleAppFireWall(cfg, req) {
		t.Fatal("sql rule should leave the request allowed")
	}
	if req.Body.String() != body {
		t.Fatal("sql rule should leave the body unchanged")
	}
}

func TestXssRejectsScriptAndAllowsPlainJSON(t *testing.T) {
	if !Blocks("xss") {
		t.Fatal("xss should be a blocking rule")
	}

	bad := jsonRequest(t, `{"name":"<script>alert(1)</script>"}`)
	if HandleXss(bad) {
		t.Fatal("script payload should be rejected")
	}
	if strings.Contains(strings.ToLower(bad.Body.String()), "<script") {
		t.Fatal("rejected body should not keep the script tag")
	}

	goodBody := `{"name":"hello"}`
	good := jsonRequest(t, goodBody)
	if !HandleXss(good) {
		t.Fatal("plain json should be allowed")
	}
	if !strings.Contains(good.Body.String(), "hello") {
		t.Fatal("plain json value should stay in the body")
	}
}

func jsonRequest(t *testing.T, body string) *request.Request {
	t.Helper()
	req := request.RequestInit(false)
	req.Method = "POST"
	req.Headers["Content-Type"] = "application/json"
	req.Headers["Content-Length"] = strconv.Itoa(len(body))
	req.Body.WriteString(body)
	return req
}
