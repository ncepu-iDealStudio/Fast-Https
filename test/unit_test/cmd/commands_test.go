package cmd_test

import (
	"fast-https/cmd"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRootCmd tests the root command initialization and basic functionality
func TestRootCmd(t *testing.T) {
	rootCmd := cmd.RootCmd()
	assert.NotNil(t, rootCmd, "Root command should not be nil")
	assert.Equal(t, "fast-https", rootCmd.Use, "Command name should be 'fast-https'")
	assert.True(t, rootCmd.SilenceUsage, "SilenceUsage should be true")
}

// TestRootCmdUnknownCommand verifies an unknown command is reported as an
// error, which makes the process exit with status 1.
func TestRootCmdUnknownCommand(t *testing.T) {
	rootCmd := cmd.RootCmd()
	rootCmd.SetArgs([]string{"__unknown__"})
	err := rootCmd.Execute()
	if assert.Error(t, err, "Unknown command should return an error") {
		assert.Contains(t, err.Error(), `unknown command "__unknown__"`)
	}
}
