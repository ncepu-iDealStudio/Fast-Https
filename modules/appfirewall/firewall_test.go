package appfirewall

import (
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"strconv"
	"strings"
	"testing"
)

// sql 规则现在具备拦截能力：body 里的注入特征会被拒绝
func TestSqlRuleInterceptsInjection(t *testing.T) {
	if !Blocks("sql") {
		t.Fatal("sql should be a blocking rule now")
	}
	req := request.RequestInit(false)
	req.Method = "POST"
	body := `{"q":"1' OR '1'='1"}`
	req.Headers["Content-Type"] = "application/json"
	req.Headers["Content-Length"] = strconv.Itoa(len(body))
	req.Body.WriteString(body)

	if HandleSql(req) {
		t.Fatal("injection in body should be rejected")
	}
	cfg := &listener.ListenCfg{AppFireWall: []string{"sql"}}
	if HandleAppFireWall(cfg, req) {
		t.Fatal("sql rule should reject the injected request")
	}

	clean := request.RequestInit(false)
	clean.Method = "POST"
	cleanBody := `{"q":"2026-10-25"}`
	clean.Headers["Content-Type"] = "application/json"
	clean.Headers["Content-Length"] = strconv.Itoa(len(cleanBody))
	clean.Body.WriteString(cleanBody)
	if !HandleAppFireWall(cfg, clean) {
		t.Fatal("clean request should pass the sql rule")
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

func TestXssStripsScriptInJSONArray(t *testing.T) {
	objects := jsonRequest(t, `[{"name":"<script>alert(1)</script>","ok":"keep"}]`)
	if HandleXss(objects) {
		t.Fatal("script inside a json array of objects should be rejected")
	}
	if strings.Contains(strings.ToLower(objects.Body.String()), "<script") {
		t.Fatal("array object should not keep the script tag")
	}
	if !strings.Contains(objects.Body.String(), "keep") {
		t.Fatal("non-script field should stay in the array")
	}

	values := jsonRequest(t, `["<script>alert(1)</script>","plain"]`)
	if HandleXss(values) {
		t.Fatal("script inside a json array of strings should be rejected")
	}
	if strings.Contains(strings.ToLower(values.Body.String()), "<script") {
		t.Fatal("array string should not keep the script tag")
	}
	if !strings.Contains(values.Body.String(), "plain") {
		t.Fatal("plain array value should stay")
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
