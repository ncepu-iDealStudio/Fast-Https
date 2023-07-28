# How to build monitor
1. add file `monitor.rc` to this dictionary, and set icon by adding this code: `IDI_ICON1 ICON "../output/icon/starting.ico"`
2. run command `windres -o monitor.syso monitor.rc`
3. then run go build command `go build -ldflags "-s -w -H=windowsgui" .`
