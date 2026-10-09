package server_test

import (
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

// TestServerSignalHandlingStop tests that SIGTERM stops the server on every platform.
func TestServerSignalHandlingStop(t *testing.T) {
	s := server.ServerInit()

	s.Wg.Add(1)
	s.SigHandler(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		s.Wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("waitgroup should be done after SIGTERM")
	}
}

// TestServerReload tests server reload functionality
func TestServerReload(t *testing.T) {
	t.Skip("reload integration depends on workspace cwd and runtime config files")
}

func TestConnectionHandling(t *testing.T) {
	t.Skip("connection-level integration test not implemented in current baseline")
}
