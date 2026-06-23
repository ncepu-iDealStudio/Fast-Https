package server_test

import (
	"fast-https/config"
	"fast-https/modules/core/server"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestServerInit tests server initialization
func TestServerInit(t *testing.T) {
	s := server.ServerInit()
	assert.NotNil(t, s, "Server instance should not be nil")
}

// TestScanPorts tests port scanning functionality
func TestScanPorts(t *testing.T) {
	// Test with free ports
	err := server.ScanPorts()
	assert.Nil(t, err, "Port scanning should succeed on fresh ports")

	// Test with occupied ports
	// TODO: Mock port occupation
}

// TestServerSignalHandling tests server signal handling
func TestServerSignalHandlingWindowsBranch(t *testing.T) {
	s := server.ServerInit()
	oldOS := config.GOs
	config.GOs = "windows"
	defer func() { config.GOs = oldOS }()

	s.Wg.Add(1)
	s.SigHandler(syscall.SIGINT)

	done := make(chan struct{})
	go func() {
		s.Wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(1 * time.Second):
		t.Fatal("waitgroup should be done after SIGINT in windows branch")
	}
}

// TestServerReload tests server reload functionality
func TestServerReload(t *testing.T) {
	t.Skip("reload integration depends on workspace cwd and runtime config files")
}

func TestConnectionHandling(t *testing.T) {
	t.Skip("connection-level integration test not implemented in current baseline")
}
