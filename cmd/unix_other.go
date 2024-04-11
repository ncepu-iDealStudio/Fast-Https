//go:build !linux || !amd64

package cmd

func Daemon(nochdir, noclose int) int {
	return 0
}
