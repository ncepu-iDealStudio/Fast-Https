package proxy

import (
	"path/filepath"
	"testing"
	"time"

	"fast-https/config"
)

func TestHealthUsesRecentWindow(t *testing.T) {
	ResetMetricsForTest()
	t.Cleanup(ResetMetricsForTest)
	t.Setenv("FASTHTTPS_UPSTREAM_DRAIN_FILE", filepath.Join(t.TempDir(), "drained.json"))
	now := time.Date(2026, 11, 14, 1, 0, 0, 0, time.UTC)
	healthNow = func() time.Time { return now }
	t.Cleanup(func() { healthNow = time.Now })

	cfg := config.Current()
	t.Cleanup(func() { config.Store(cfg, nil) })
	config.Store(&config.Fast_Https{Servers: []config.Server{{
		Path: []config.Path{{
			ProxyPeers: []string{"10.0.0.1:8080", "10.0.0.2:8080", "10.0.0.3:8080", "10.0.0.4:8080"},
		}},
	}}}, nil)

	noteUpstreamOK("10.0.0.1:8080")
	noteDialFail("10.0.0.2:8080")
	noteDialFail("10.0.0.3:8080")
	if err := SaveDrained([]string{"10.0.0.2:8080"}); err != nil {
		t.Fatal(err)
	}

	got := healthByAddr(t)
	if got["10.0.0.1:8080"].Health != "healthy" || !got["10.0.0.1:8080"].Healthy {
		t.Fatalf("recent success = %+v", got["10.0.0.1:8080"])
	}
	if got["10.0.0.2:8080"].Health != "unhealthy" || !got["10.0.0.2:8080"].Drained {
		t.Fatalf("drained failure = %+v", got["10.0.0.2:8080"])
	}
	if got["10.0.0.4:8080"].Health != "unknown" || got["10.0.0.4:8080"].Healthy {
		t.Fatalf("no samples = %+v", got["10.0.0.4:8080"])
	}

	healthNow = func() time.Time { return now.Add(2 * time.Minute) }
	noteUpstreamOK("10.0.0.3:8080")
	got = healthByAddr(t)
	if got["10.0.0.1:8080"].Health != "unknown" {
		t.Fatalf("success outside the window = %+v", got["10.0.0.1:8080"])
	}
	if got["10.0.0.2:8080"].Health != "unknown" || got["10.0.0.2:8080"].DialFailTotal != 1 || !got["10.0.0.2:8080"].Drained {
		t.Fatalf("old failure = %+v", got["10.0.0.2:8080"])
	}
	if got["10.0.0.3:8080"].Health != "healthy" {
		t.Fatalf("recovered = %+v", got["10.0.0.3:8080"])
	}
}

func TestRecentFailureIsUnhealthy(t *testing.T) {
	ResetMetricsForTest()
	t.Cleanup(ResetMetricsForTest)
	now := time.Date(2026, 11, 14, 2, 0, 0, 0, time.UTC)
	healthNow = func() time.Time { return now }
	t.Cleanup(func() { healthNow = time.Now })
	noteDialFail("10.1.0.1:8080")
	noteUpstreamOK("10.1.0.2:8080")
	got := healthByAddr(t)
	if got["10.1.0.1:8080"].Health != "unhealthy" || got["10.1.0.2:8080"].Health != "healthy" {
		t.Fatalf("health = %+v", got)
	}
}

func healthByAddr(t *testing.T) map[string]AddrStats {
	t.Helper()
	st, _ := Snapshot()
	got := map[string]AddrStats{}
	for _, row := range st.Upstreams {
		got[row.Address] = row
	}
	return got
}
