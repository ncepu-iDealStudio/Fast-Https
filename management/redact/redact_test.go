package redact

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMasksKnownSecretKeys(t *testing.T) {
	in := decode(t, `{
		"http": {"server": [{"location": [{"auth": {"type": "basic", "user": "admin", "pswd": "S3cr3t-XYZ", "realm": "r"}}]}]},
		"acme": {"cas": [{"name": "zerossl", "eab_kid": "KID-1", "eab_hmac_key": "HMAC-ABC"}]},
		"db_password": "p1", "client_secret": "s1", "access_token": "t1"
	}`)

	out, _ := json.Marshal(Value(in))
	got := string(out)
	for _, leaked := range []string{"S3cr3t-XYZ", "KID-1", "HMAC-ABC", `"p1"`, `"s1"`, `"t1"`} {
		if strings.Contains(got, leaked) {
			t.Errorf("%s 没有被遮蔽：%s", leaked, got)
		}
	}
	for _, kept := range []string{"admin", "zerossl", "basic"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%s 不是敏感值，不该被遮蔽：%s", kept, got)
		}
	}
}

func TestKeepsPathsAndPlainKeys(t *testing.T) {
	in := decode(t, `{"ssl_certificate_key": "./cert/a-key.pem", "key": "$uri", "max_body_size": 10}`)
	out := Value(in)
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("不应改动：%v -> %v", in, out)
	}
}

func TestEmptyAndNullSecretsStayVisible(t *testing.T) {
	out := Value(decode(t, `{"pswd": "", "token": null}`)).(map[string]any)
	if out["pswd"] != "" || out["token"] != nil {
		t.Fatalf("空值应保持原样，便于看出没配：%v", out)
	}
}

func TestProxyHeaderValueMaskedByHeaderName(t *testing.T) {
	in := decode(t, `[
		{"HeaderKey": "Authorization", "HeaderValue": "Bearer abc"},
		{"HeaderKey": "X-Real-IP", "HeaderValue": "$remote_addr"}
	]`)
	got, _ := json.Marshal(Value(in))
	s := string(got)
	if strings.Contains(s, "Bearer abc") {
		t.Fatalf("Authorization 的值没有被遮蔽：%s", s)
	}
	if !strings.Contains(s, "$remote_addr") {
		t.Fatalf("普通头不应被遮蔽：%s", s)
	}
}

func TestDoesNotMutateInput(t *testing.T) {
	in := decode(t, `{"pswd": "keep-me"}`).(map[string]any)
	_ = Value(in)
	if in["pswd"] != "keep-me" {
		t.Fatal("输入被改写")
	}
}
