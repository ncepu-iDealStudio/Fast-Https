package main

// 开机自启的注册表读写（HKCU\...\Run）：桌面端唯一的持久化系统设置。

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestAutoStartRoundTrip(t *testing.T) {
	s := &DesktopService{baseDir: t.TempDir()}
	// 初始：未配置
	on, err := s.GetAutoStart()
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("测试机不应预先配置自启（若真配置了请先在界面取消）")
	}
	// 开启
	on, err = s.SetAutoStart(true)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("SetAutoStart(true) 后 GetAutoStart 应为 true")
	}
	// 注册表里的命令行必须带 --tray（开机只进托盘不弹窗）
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Fast-Https Desktop")
	if err != nil {
		t.Fatalf("读 Run 值失败：%v", err)
	}
	if !strings.Contains(v, "--tray") {
		t.Errorf("自启命令行应带 --tray（托盘常驻），得到 %q", v)
	}
	if !strings.Contains(v, "fast-https-desktop") {
		t.Errorf("自启命令行应指向桌面端 exe，得到 %q", v)
	}
	// 关闭
	on, err = s.SetAutoStart(false)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("SetAutoStart(false) 后 GetAutoStart 应为 false")
	}
}

// TestAutostartCommandFormat 锁住命令行格式：带引号的 exe 路径 + --tray。
// 引号不能少：Program Files 路径带空格，没引号会被 Run 键拆错参数。
func TestAutostartCommandFormat(t *testing.T) {
	cmd, err := autostartCommand()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cmd, `"`) || !strings.HasSuffix(cmd, `" --tray`) {
		t.Errorf("自启命令行应为 %q 形式，得到 %q", `"exe路径" --tray`, cmd)
	}
}
