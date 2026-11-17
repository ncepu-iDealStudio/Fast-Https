package proxyconf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fast-https/config/internal/model"
)

func TestWriteProxySectionKeepsComments(t *testing.T) {
	path := writeProxyFixture(t)

	preserve := false
	want := model.ProxyConfig{
		TrustedProxies: []string{"10.0.0.0/8", "192.168.1.10"},
		ForwardedFor:   model.ForwardedForConfig{MaxHops: 8},
		PreserveHost:   &preserve,
	}
	if err := WriteProxySection(path, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "// keep me") {
		t.Fatalf("注释丢了:\n%s", raw)
	}
	got, err := ReadProxySection(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TrustedProxies) != 2 || got.ForwardedFor.MaxHops != 8 || got.PreserveHostEnabled() {
		t.Fatalf("读回不一致: %+v", got)
	}
}

func TestWriteProxySectionDeletesDefaultSection(t *testing.T) {
	path := writeProxyFixture(t)
	if err := WriteProxySection(path, model.ProxyConfig{
		TrustedProxies: []string{"10.0.0.0/8"},
		ForwardedFor:   model.ForwardedForConfig{MaxHops: 2},
	}); err != nil {
		t.Fatal(err)
	}

	if err := WriteProxySection(path, model.ProxyConfig{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"proxy"`) {
		t.Fatalf("默认值应删除整段:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"pid": "run.pid"`) || !strings.Contains(string(raw), "// keep me") {
		t.Fatalf("其它字段或注释被破坏:\n%s", raw)
	}
	got, err := ReadProxySection(path)
	if err != nil {
		t.Fatal(err)
	}
	if !IsDefault(got) {
		t.Fatalf("删段后应读回零值: %+v", got)
	}
}

func TestPreviewProxySectionRejectsBrokenJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.json")
	if err := os.WriteFile(path, []byte(`{"pid": `), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PreviewProxySection(path, model.ProxyConfig{ForwardedFor: model.ForwardedForConfig{MaxHops: 1}}); err == nil {
		t.Fatal("坏配置应报错")
	}
}

func writeProxyFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fast-https.json")
	body := `{
  // keep me
  "pid": "run.pid",
  "http": {"server": []}
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
