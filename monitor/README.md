# Fast-Https 托盘程序

Windows 任务栏托盘程序，用来在本机启动和停止 Fast-Https。它是 `monitor/` 下的独立程序，只支持 **windows/amd64**（`monitor.go` 与 `windows.go` 带有 `//go:build windows && amd64`）。界面使用 `fyne.io/systray`。

托盘程序自己不监听端口。它在**可执行文件所在目录**里查找并调用旁边的 `fast-https.exe`，日志目录与 pid 文件也按该目录解析，所以从任意工作目录启动都可以，换目录启动不会再认错文件。

## 单实例

托盘是单实例的：`logs/monitor.lock` 以独占方式打开，第二个实例拿不到句柄，会记一条日志后直接退出，不会出现多个托盘图标各自启停同一个服务。

锁靠文件句柄维持，进程结束（包括崩溃）时由内核释放，残留的 `monitor.lock` 不影响下次启动，不需要手工删除。

## 菜单

右键托盘图标可以看到三项：

| 菜单 | 动作 |
| --- | --- |
| Start | 隐藏窗口执行 `fast-https`。没有参数时主程序走 `start`，即普通启动，不是 `dev`。 |
| Stop | 隐藏窗口执行 `fast-https stop`。 |
| Quit | 退出托盘。**由本托盘启动的服务器会被一起停止**，最长等待 10 秒；不是本托盘启动的（例如系统服务方式启动的实例）保持不动，只记一条日志。 |

菜单状态由真实进程状态驱动，每 2 秒同步一次：

- 服务器在跑：图标换成运行中，Start 置灰，Stop 可用。
- 服务器已停：图标换成已停止，Start 可用，Stop 置灰。

因为状态来自真实进程，服务在托盘之外被停掉或自己崩溃时，菜单会在下一个周期自动纠正，不会卡在"运行中"。

`windows.go` 里有 `reloadServer`，它会执行 `fast-https reload`，但菜单没有接入这一项。

## 启动时如何判断服务器是否在跑

读取 `fast-https.pid`，用 `OpenProcess` 拿句柄后再调 `GetExitCodeProcess`，只有退出码为 `STILL_ACTIVE`（259）才算在运行。

只判断 `OpenProcess` 是否成功是不够的：进程已经退出但内核对象还没回收时，该调用依然会成功，残留的 pid 文件就会被误判成"运行中"并禁用 Start。

## 日志与图标

托盘自己的日志，以及它拉起的主程序标准输出，都追加写入程序目录下的 `logs/monitor.log`。目录来自 `config.DEFAULT_LOG_ROOT`（`./logs`），文件名是 `monitor.log`。

托盘图标是 `icon.go` 里的两套内嵌图标：运行中为 `LogoExecuting`，已停止为 `LogoStopping`。

`monitor.rc` 另有一行 `IDI_ICON1 ICON "../output/icon/starting.ico"`，这是资源管理器里 `monitor.exe` 的文件图标，不是托盘图标。仓库里目前没有 `output/icon/starting.ico`。发布流程不调用 `windres`，因此缺少这个文件不影响托盘和安装包。

## 编译

在仓库根目录执行。`-H=windowsgui` 让程序以无控制台窗口的方式运行。

```bash
go build -ldflags "-s -w -H=windowsgui" -o monitor/monitor.exe ./monitor
```

`build/goreleaser.yaml` 的 `before` 钩子会用同一条命令编译 `monitor/monitor.exe`，再放进 Windows zip 的根目录，和 `fast-https.exe` 放在一起。钩子编译失败时，这次发布会停下来。

## 测试

`windows_test.go` 覆盖路径解析、单实例锁与非法 pid 判定：

```bash
go test ./monitor/
```
