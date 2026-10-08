package cmd

import (
	"os"
	"testing"
)

func TestRunInForegroundFromEnv(t *testing.T) {
	t.Setenv("FASTHTTPS_FOREGROUND", "1")
	if !runInForeground() {
		t.Fatal("FASTHTTPS_FOREGROUND=1 should keep the server in the foreground")
	}
}

func TestRunInForegroundUnset(t *testing.T) {
	t.Setenv("FASTHTTPS_FOREGROUND", "")
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("container marker exists in this environment")
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		t.Skip("container marker exists in this environment")
	}
	if runInForeground() {
		t.Fatal("empty FASTHTTPS_FOREGROUND should daemonize outside a container")
	}
}
