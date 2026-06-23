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

// TestRootCmdUnknownCommand verifies unknown command path is non-fatal.
func TestRootCmdUnknownCommand(t *testing.T) {
	rootCmd := cmd.RootCmd()
	rootCmd.SetArgs([]string{"__unknown__"})
	err := rootCmd.Execute()
	assert.NoError(t, err, "Unknown command should return nil and print usage hint")
}
