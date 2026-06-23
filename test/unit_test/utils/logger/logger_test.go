package logger_test

import (
	"fast-https/test/unit_test/helpers"
	"fast-https/utils/logger"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var mockIO *helpers.MockIO

func TestMain(m *testing.M) {
	mockIO = helpers.NewMockIO()
	os.Exit(m.Run())
}

func captureCombinedOutput(run func()) string {
	mockIO.Stdout.Reset()
	mockIO.Stderr.Reset()
	cleanup := mockIO.CaptureOutput()
	run()
	cleanup()
	return mockIO.Stdout.String() + mockIO.Stderr.String()
}

// TestLoggerLevel tests logger level setting and getting
func TestLoggerLevel(t *testing.T) {
	testCases := []struct {
		level    int
		expected string
	}{
		{0, "FATAL"},
		{1, "ERROR"},
		{2, "WARN"},
		{3, "NOTICE"},
		{4, "INFO"},
		{5, "DEBUG"},
		{6, "TRACE"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			logger.Level(tc.level)
			output := captureCombinedOutput(func() {
				logger.Info("test message")
			})

			if tc.level >= 4 {
				assert.Contains(t, output, "test message", "Message should be logged")
			} else {
				assert.Empty(t, output, "Message should not be logged")
			}
		})
	}
}

// TestLogOutput tests different types of log output
func TestLogOutput(t *testing.T) {
	testCases := []struct {
		name     string
		logFunc  func(string, ...interface{})
		level    int
		message  string
		expected string
	}{
		{"Error", logger.Error, 1, "test error", "ERROR"},
		{"Warn", logger.Warn, 2, "test warn", "WARN"},
		{"Notice", logger.Notice, 3, "test notice", "NOTICE"},
		{"Info", logger.Info, 4, "test info", "INFO"},
		{"Debug", logger.Debug, 5, "test debug", "DEBUG"},
		{"Trace", logger.Trace, 6, "test trace", "TRACE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger.Level(6) // Set to highest level to capture all logs
			output := captureCombinedOutput(func() {
				tc.logFunc(tc.message)
			})

			assert.True(t, strings.Contains(output, tc.expected),
				"Log output should contain correct level")
			assert.True(t, strings.Contains(output, tc.message),
				"Log output should contain message")
		})
	}
}

// TestFatalLog validates that logger.Fatal terminates the process with non-zero code.
func TestFatalLog(t *testing.T) {
	if os.Getenv("LOGGER_FATAL_SUBPROCESS") == "1" {
		logger.Level(0)
		logger.Fatal("test fatal message")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalLog")
	cmd.Env = append(os.Environ(), "LOGGER_FATAL_SUBPROCESS=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("logger.Fatal should terminate subprocess with non-zero exit code")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatal("logger.Fatal subprocess exit code should be non-zero")
	}
}

// TestLogFormatting tests log message formatting
func TestLogFormatting(t *testing.T) {
	logger.Level(6)
	output := captureCombinedOutput(func() {
		logger.Info("test %s %d", "message", 123)
	})

	assert.Contains(t, output, "test message 123",
		"Formatted message should be correct")
}
