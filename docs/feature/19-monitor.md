# 19. 托盘监控（monitor）

状态：✅ 已实现

## 功能

Windows 系统托盘程序：Start/Stop/Quit 菜单控制主服务，图标随服务状态切换。

## 使用

```
fast-https-monitor.exe   # 独立二进制
```

## 实现方案（monitor/）

- `fyne.io/systray`（getlantern/systray 的活跃维护分支）渲染托盘与菜单。
- 通过 `os/exec` 拉起/停止主程序二进制，轮询状态切换 `SetTemplateIcon`。
- 运行日志落 `monitor.log`。

## 测试

无自动化测试（GUI 组件），依赖手工验证；`go build ./...` 覆盖编译。

## 已知限制

- 仅 Windows（构建标签 `windows && amd64`）。
- 图标数据（`icon.go` 约 9000 行生成字节）拖慢编译，可外置资源文件。
