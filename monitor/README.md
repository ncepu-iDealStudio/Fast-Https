# Fast-Https 托盘程序

Windows 任务栏托盘程序，用来在本机启动和停止 Fast-Https。它是 `monitor/` 下的独立程序，只支持 **windows/amd64**（`monitor.go` 与 `windows.go` 带有 `//go:build windows && amd64`）。界面使用 `github.com/getlantern/systray`。

托盘程序自己不监听端口。它在当前工作目录里调用旁边的主程序 `fast-https.exe`。请在主程序所在目录启动它；从其它目录启动时，会找不到主程序、pid 文件和日志。

## 菜单

右键托盘图标可以看到三项：

| 菜单 | 动作 |
| --- | --- |
| Start | 隐藏窗口执行 `fast-https`。没有参数时主程序走 `start`，即普通启动，不是 `dev`。 |
| Stop | 隐藏窗口执行 `fast-https stop`。 |
| Quit | 只退出托盘。已经启动的服务器继续运行。 |

点 Start 后，Start 变为不可用，托盘图标换成运行中的图标。点 Stop 后，Start 恢复可用，图标换成已停止的图标。

`windows.go` 里有 `reloadServer`，它会执行 `fast-https reload`，但菜单没有接入这一项。

## 启动时如何判断服务器是否在跑

启动时读取当前目录的 `fast-https.pid`。

- 读不到文件，或文件为空：显示已停止图标，Start 可以点击。
- 读到进程号：调用 `os.FindProcess`。在 Windows 上，只要进程号是合法数字，这个调用就会成功，并不确认进程仍在运行。因此残留的 `fast-https.pid` 也会显示为运行中，并禁用 Start。

## 日志与图标

托盘自己的日志，以及它拉起的主程序标准输出，都追加写入当前目录的 `logs/monitor.log`。目录来自 `config.DEFAULT_LOG_ROOT`（`./logs`），文件名是 `monitor.log`。

托盘图标是 `icon.go` 里的两套内嵌图标：运行中为 `LogoExecuting`，已停止为 `LogoStopping`。

`monitor.rc` 另有一行 `IDI_ICON1 ICON "../output/icon/starting.ico"`，这是资源管理器里 `monitor.exe` 的文件图标，不是托盘图标。仓库里目前没有 `output/icon/starting.ico`。没有这个文件时，下面的 `windres` 会失败；托盘图标不依赖它。

## 编译

在 `monitor/` 目录执行。`-H=windowsgui` 让程序以无控制台窗口的方式运行。

```bash
windres -o monitor.syso monitor.rc
go build -ldflags "-s -w -H=windowsgui" -o monitor.exe .
```

Windows 发布包（`.goreleaser.windows.yaml`）会把编好的 `monitor/monitor.exe` 放进 zip 的根目录，和 `fast-https.exe` 放在一起。打 Windows 包之前需要先完成本节的编译。
