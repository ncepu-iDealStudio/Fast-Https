//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func fileIdentity(f *os.File) (string, bool) {
	var info syscall.ByHandleFileInformation
	err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info)
	if err != nil || (info.FileIndexHigh == 0 && info.FileIndexLow == 0) {
		return "", false
	}
	return fmt.Sprintf("id:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), true
}
