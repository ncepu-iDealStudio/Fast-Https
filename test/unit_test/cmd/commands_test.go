package cmd_test

import (
	"fast-https/cmd"
	"fast-https/test/unit_test/helpers"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	mockProc *helpers.MockProcessManager
	mockIO   *helpers.MockIO
)

func TestMain(m *testing.M) {
	// Setup
	mockProc = helpers.NewMockProcessManager()
	mockIO = helpers.NewMockIO()
	
	// Run tests
	code := m.Run()
	
	// Cleanup
	os.Exit(code)
}

// TestRootCmd tests the root command initialization and basic functionality
func TestRootCmd(t *testing.T) {
	rootCmd := cmd.RootCmd()
	assert.NotNil(t, rootCmd, "Root command should not be nil")
	assert.Equal(t, "fast-https", rootCmd.Use, "Command name should be 'fast-https'")
	
	// Test command flags
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("start"), "Start flag should exist")
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("stop"), "Stop flag should exist")
	assert.NotNil(t, rootCmd.PersistentFlags().Lookup("reload"), "Reload flag should exist")
}

// TestStartHandler tests the start command handler
func TestStartHandler(t *testing.T) {
	// Setup test environment
	cleanup := mockIO.CaptureOutput()
	defer cleanup()

	err := cmd.StartHandler()
	assert.Nil(t, err, "Start handler should not return error")
	
	// Verify output contains expected startup messages
	output := mockIO.Stdout.String()
	assert.Contains(t, output, "server start", "Should show startup message")
}

// TestStopHandler tests the stop command handler
func TestStopHandler(t *testing.T) {
	// Create mock process
	mockProc.CreateProcess(1234)
	
	// Setup test PID file
	mockIO.WriteFile("pid", []byte(`{"pid": 1234}`), 0644)
	
	err := cmd.StopHandler()
	assert.Nil(t, err, "Stop handler should not return error")
}

// TestReloadHandler tests the reload command handler
func TestReloadHandler(t *testing.T) {
	// Create mock process
	mockProc.CreateProcess(1234)
	
	// Setup test PID file
	mockIO.WriteFile("pid", []byte(`{"pid": 1234}`), 0644)
	
	err := cmd.ReloadHandler()
	assert.Nil(t, err, "Reload handler should not return error")
	
	// Verify signal was sent
	proc, _ := mockProc.FindProcess(1234)
	assert.Equal(t, os.Interrupt, proc.Signal, "Should send interrupt signal")
}

// TestServiceInstallHandler tests the service installation handler
func TestServiceInstallHandler(t *testing.T) {
	cleanup := mockIO.CaptureOutput()
	defer cleanup()

	err := cmd.ServiceInstallHandler()
	assert.Nil(t, err, "Service installation should not return error")
	
	output := mockIO.Stdout.String()
	assert.Contains(t, output, "service installed", "Should show installation message")
}

// TestServiceUnInstallHandler tests the service uninstallation handler
func TestServiceUnInstallHandler(t *testing.T) {
	cleanup := mockIO.CaptureOutput()
	defer cleanup()

	err := cmd.ServiceUnInstallHandler()
	assert.Nil(t, err, "Service uninstallation should not return error")
	
	output := mockIO.Stdout.String()
	assert.Contains(t, output, "uninstalled", "Should show uninstallation message")
} 