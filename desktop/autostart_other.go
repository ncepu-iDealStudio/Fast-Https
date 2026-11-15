//go:build !windows

package main

// 非 Windows 平台的开机自启桩：桌面端当前只发行 Windows 安装包，
// Linux 走 deb/rpm/apk 系统包（systemd/user 服务另有机制），这里
// 保持 Wails bindings 方法集合不变（恒为未配置、设置无效）。

func autostartCommand() (string, error) { return "", nil }

func (s *DesktopService) GetAutoStart() (bool, error) { return false, nil }

func (s *DesktopService) SetAutoStart(bool) (bool, error) { return false, nil }
