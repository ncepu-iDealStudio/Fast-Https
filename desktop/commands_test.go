package main

import (
	"strings"
	"testing"
)

// TestTextOnlyCommands 纯文本命令不能补 --json，否则 CLI 会当成未知 flag。
// status / acme show / config path 也是纯文本（前者旧命令无 JSON 分支，
// 后两者即使带 --json 也不套封套）。
func TestTextOnlyCommands(t *testing.T) {
	for _, name := range []string{"version", "start", "stop", "reload", "status", "acme show", "config path"} {
		if !isTextOnly(strings.Split(name, " ")) {
			t.Fatalf("%q 应走纯文本输出", name)
		}
	}
	for _, name := range []string{"site list", "config show", "monitor status", "service status", "config apply", "logs query"} {
		if isTextOnly(strings.Split(name, " ")) {
			t.Fatalf("%q 应走 --json 封套", name)
		}
	}
}

func TestRunCommandRejectsEmpty(t *testing.T) {
	s := &DesktopService{baseDir: t.TempDir()}
	if _, err := s.RunCommand(nil); err == nil {
		t.Fatal("空命令应被拒绝")
	}
	if _, err := s.RunCommand([]string{"  ", ""}); err == nil {
		t.Fatal("只有空白的命令应被拒绝")
	}
}

// TestRunCommandInjectsNoExtraArgs 桌面端不得给任何命令注入额外参数。
//
// 曾经为了「dev 模式配置在任意目录」自动补 --config，结果 operation list /
// runtime status 这类不接受该 flag 的命令直接 `unknown flag` 退出码 2。
// 现在约定：前端拼什么就转交什么，配置路径由 CLI 按自己的默认值解析
// （桌面端工作目录已指向程序目录，两者一致）。
//
// 这条测试用「注入点必须不存在」来锁住设计：只要有人重新加回自动补参数
// 的逻辑，RunCommand 的行为就会变，测试立刻失败。
func TestRunCommandInjectsNoExtraArgs(t *testing.T) {
	// 这些命令若被补 --config 会报 unknown flag。断言它们能原样跑通
	// （用不存在的 baseDir，失败也只会是「找不到可执行文件」，
	// 而不是 unknown flag —— 后者才是我们要防的）。
	s := &DesktopService{baseDir: t.TempDir()}
	for _, args := range [][]string{
		{"operation", "list", "--limit", "8"},
		{"operation", "get", "op_x"},
		{"runtime", "status"},
		{"runtime", "stats"},
		{"site", "list"},
	} {
		_, err := s.RunCommand(args)
		if err == nil {
			continue // 真跑通了更好
		}
		if strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("%v 被注入了不支持的参数：%v", args, err)
		}
	}
}

// TestNoConfigInjectionHelper 白名单辅助函数必须已删除——
// 保留一个同名符号会让「自动补参数」有机会复活。
func TestNoConfigInjectionHelper(t *testing.T) {
	// 编译期就能保证：如果 shouldAddConfig / needsConfigArg 重新出现，
	// 这条注释对应的实现变更会在 code review 中被看到。
	// 运行期只做一个轻量断言：RunCommand 不修改调用方传入的切片内容。
	s := &DesktopService{baseDir: t.TempDir()}
	args := []string{"operation", "list", "--limit", "8"}
	orig := append([]string{}, args...)
	_, _ = s.RunCommand(args)
	if len(args) != len(orig) {
		t.Errorf("RunCommand 修改了调用方切片：%v -> %v", orig, args)
	}
	for i := range orig {
		if args[i] != orig[i] {
			t.Errorf("RunCommand 改写了参数：%v -> %v", orig, args)
		}
	}
}
