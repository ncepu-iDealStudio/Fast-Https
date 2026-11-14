package cmd

import (
	"testing"

	"fast-https/management/rtclient"
)

func TestOldHealthyFalseDoesNotAllowDrain(t *testing.T) {
	no := false
	yes := true
	rows := []rtclient.UpstreamAddrInfo{
		{Address: "10.0.0.1:8080", Healthy: &no},
		{Address: "10.0.0.2:8080", Healthy: &yes},
	}
	if err := drainStillAllowed(rows, "10.0.0.1:8080"); err == nil {
		t.Fatal("old healthy=false was treated as a drain candidate")
	}
}
