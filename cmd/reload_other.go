//go:build !windows

package cmd

func sendCtrlC(_ int) error { return nil }
