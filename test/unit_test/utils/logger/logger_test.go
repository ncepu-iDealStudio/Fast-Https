package logger_test

import (
	"fast-https/test/unit_test/helpers"
	"fast-https/utils/logger"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var mockIO *helpers.MockIO

func TestMain(m *testing.M) {
	mockIO = helpers.NewMockIO()
	m.Run()
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
			cleanup := mockIO.CaptureOutput()
			defer cleanup()

			// Test logging at current level
			logger.Info("test message")
			output := mockIO.Stdout.String()

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
			cleanup := mockIO.CaptureOutput()
			
			tc.logFunc(tc.message)
			output := mockIO.Stdout.String()
			cleanup()

			assert.True(t, strings.Contains(output, tc.expected), 
				"Log output should contain correct level")
			assert.True(t, strings.Contains(output, tc.message), 
				"Log output should contain message")
		})
	}
}

// TestFatalLog tests fatal log handling
func TestFatalLog(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Fatal log should panic")
		}
	}()

	cleanup := mockIO.CaptureOutput()
	defer cleanup()

	logger.Level(0)
	logger.Fatal("test fatal message")
}

// TestLogFormatting tests log message formatting
func TestLogFormatting(t *testing.T) {
	logger.Level(6)
	cleanup := mockIO.CaptureOutput()
	defer cleanup()

	logger.Info("test %s %d", "message", 123)
	output := mockIO.Stdout.String()

	assert.Contains(t, output, "test message 123", 
		"Formatted message should be correct")
} 