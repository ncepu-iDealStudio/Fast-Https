package certmanager

import (
	"context"
	"testing"

	"fast-https/config"
)

func TestManagedMatchesPlanName(t *testing.T) {
	cfg := &config.Fast_Https{Servers: []config.Server{{
		Listen:     "443 ssl",
		ServerName: "App.Example.",
	}}}
	if !Managed(cfg, "app.example") {
		t.Fatal("App.Example. should be managed as app.example")
	}
	if Managed(cfg, "other.example") {
		t.Fatal("other.example is outside the plan")
	}
	if Managed(cfg, "localhost") {
		t.Fatal("localhost is not an ACME name")
	}
}

func TestRenewWithoutManagerLeavesCallerToKeepFiles(t *testing.T) {
	if activeManager.Load() != nil {
		t.Skip("an ACME manager is already running in this process")
	}
	err := Renew(context.Background(), "app.example")
	if err == nil {
		t.Fatal("renew without a manager should fail before any certificate write")
	}
}
