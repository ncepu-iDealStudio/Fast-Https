package main

import "testing"

func TestInterpretHealth(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		health string
		legacy *bool
		want   string
	}{
		{"healthy", nil, "healthy"},
		{"unhealthy", &yes, "unhealthy"},
		{"unknown", nil, "unknown"},
		{"", &yes, "healthy"},
		{"", &no, "unknown"},
		{"", nil, "unknown"},
		{"bad", &no, "unknown"},
	}
	for _, tc := range cases {
		if got := interpretHealth(tc.health, tc.legacy); got != tc.want {
			t.Fatalf("health=%q legacy=%v got %s want %s", tc.health, tc.legacy, got, tc.want)
		}
	}
}
