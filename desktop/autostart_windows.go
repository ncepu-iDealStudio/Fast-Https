package main

// 开机自启动（Windows）：写 HKCU\...\Run（当前用户级，无需管理员），
// 值为桌面端自身路径，附 --tray 参数让开机只进托盘、不弹主窗口。
// 注册表是自启的事实来源：GetAutoStart 读它画勾，SetAutoStart 增删。
// 逻辑放平台分离文件：golang.org/x/sys/windows/registry 被 build
// constraints 限制在 Windows，顶层 import 会炸掉 goreleaser 的
// Linux 交叉构建（desktop deb/rpm/apk）。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValueName = "Fast-Https Desktop"

// autostartCommand 开机启动的完整命令行：exe + --tray（托盘常驻模式）。
func autostartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + exe + `" --tray`, nil
}

// GetAutoStart 报告是否已配置开机自启。
func (s *DesktopService) GetAutoStart() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false, nil // 键不存在＝没配置
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValueName)
	if err != nil {
		return false, nil
	}
	want, _ := autostartCommand()
	return strings.Contains(v, filepath.Base(strings.Trim(want, `"`))), nil
}

// SetAutoStart 设置或取消开机自启；返回当前状态。
func (s *DesktopService) SetAutoStart(enabled bool) (bool, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("打开注册表失败：%w", err)
	}
	defer k.Close()
	if enabled {
		cmd, err := autostartCommand()
		if err != nil {
			return false, fmt.Errorf("取程序路径失败：%w", err)
		}
		if err := k.SetStringValue(autostartValueName, cmd); err != nil {
			return false, fmt.Errorf("写入注册表失败：%w", err)
		}
	} else {
		// 值不存在时 DeleteValue 返回错误，删除目标本就是「没有」，忽略
		_ = k.DeleteValue(autostartValueName)
	}
	return s.GetAutoStart()
}
