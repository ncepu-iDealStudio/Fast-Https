//go:build !unix && !windows

package main

import "os"

func fileIdentity(*os.File) (string, bool) { return "", false }
