package main

import "testing"

func TestDrainStepHealthCases(t *testing.T) {
	cases := []struct {
		name string
		ctx  string
		want string
	}{
		{"all healthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"healthy"},{"address":"10.0.0.2:8080","health":"healthy"}]}}}`, ""},
		{"one unhealthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"healthy"}]}}}`, "upstream drain 10.0.0.1:8080"},
		{"all unhealthy", `{"runtime":{"upstream":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"unhealthy"}]}}}`, ""},
		{"recovered", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"healthy","healthy":false},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
		{"no samples", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unknown"},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
		{"already drained", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy","drained":true},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
		{"only one", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"}]}}`, ""},
		{"one healthy among several", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"unknown"},{"address":"10.0.0.3:8080","health":"healthy"}]}}`, "upstream drain 10.0.0.1:8080"},
		{"legacy false is unknown", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","healthy":false},{"address":"10.0.0.2:8080","healthy":true}]}}`, ""},
		{"legacy true only", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","healthy":true},{"address":"10.0.0.2:8080","healthy":true}]}}`, ""},
		{"missing health fields", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080"},{"address":"10.0.0.2:8080"}]}}`, ""},
		{"illegal health", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"bad"},{"address":"10.0.0.2:8080","health":"healthy"}]}}`, ""},
		{"unknown among peers", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unhealthy"},{"address":"10.0.0.2:8080","health":"unknown"}]}}`, ""},
		{"all unknown", `{"runtime":{"upstreams":[{"address":"10.0.0.1:8080","health":"unknown"},{"address":"10.0.0.2:8080","health":"unknown"}]}}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := drainStep("http_5xx_spike", tc.ctx)
			text := ""
			if len(got) == 1 {
				text = got[0]
			} else if len(got) > 1 {
				t.Fatalf("steps = %v", got)
			}
			if text != tc.want {
				t.Fatalf("step = %q, want %q", text, tc.want)
			}
		})
	}
}

func TestPlanHashChangesWithExecutionFields(t *testing.T) {
	steps := []string{"upstream drain 10.0.0.1:8080"}
	base := hashPlan("e1", "low", planVerifyText, planOnFailureText, steps)
	if hashPlan("e1", "low", planVerifyText, planOnFailureText, steps) != base {
		t.Fatal("same plan produced two hashes")
	}
	if hashPlan("e1", "high", planVerifyText, planOnFailureText, steps) == base {
		t.Fatal("risk change did not change the hash")
	}
	if hashPlan("e1", "low", "other", planOnFailureText, steps) == base {
		t.Fatal("verify change did not change the hash")
	}
	if hashPlan("e1", "low", planVerifyText, "stop", steps) == base {
		t.Fatal("failure strategy change did not change the hash")
	}
}
