package metrics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSectionRegistration(t *testing.T) {
	ResetForTest()

	raw, err := json.Marshal(TakeSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"upstream"`) {
		t.Fatalf("unregistered section should be omitted: %s", raw)
	}

	RegisterSection("upstream", func() (any, bool) {
		return map[string]uint64{"dial_fail_total": 3}, true
	})
	RegisterSection("empty", func() (any, bool) { return nil, false })

	raw, err = json.Marshal(TakeSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	up, ok := m["upstream"].(map[string]any)
	if !ok {
		t.Fatalf("upstream section missing: %s", raw)
	}
	if up["dial_fail_total"] != float64(3) {
		t.Fatalf("upstream = %#v", up)
	}
	if _, present := m["empty"]; present {
		t.Fatal("section that returned ok=false must be omitted")
	}
}

func TestSnapshotOmitsUnmeasuredReload(t *testing.T) {
	ResetForTest()
	raw, err := json.Marshal(TakeSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "last_reload_at") || strings.Contains(string(raw), "last_reload_ok") {
		t.Fatalf("never-reloaded snapshot should omit reload result: %s", raw)
	}
	if !strings.Contains(string(raw), "request_duration") {
		t.Fatalf("request_duration is measured from process start and must appear: %s", raw)
	}
}

func TestUpstreamSectionOmittedWithoutProxy(t *testing.T) {
	ResetForTest()
	raw, err := json.Marshal(TakeSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"upstream"`) {
		t.Fatalf("unregistered upstream must be omitted: %s", raw)
	}
}
