1. run command `windres -o monitor.syso monitor.rc`
2. then run go build command `go build -ldflags "-s -w -H=windowsgui  -o monitor.exe monitor.go`