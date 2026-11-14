//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(f *os.File) (string, bool) {
	info, err := f.Stat()
	if err != nil {
		return "", false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		return "", false
	}
	return fmt.Sprintf("id:%d:%d", st.Dev, st.Ino), true
}
