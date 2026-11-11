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

// TestShouldAddConfig 锁定「哪些命令自动补 --config」：
// dev 模式下配置可能被放在任意目录（例如 desktop/bin/config），
// 不能依赖 CLI 进程的 CWD 找 ./config/fast-https.json。
//
// 判断依据是「该命令的 --help 里有没有 --config」。误加会让命令报
// `unknown flag: --config` 并以退出码 2 失败。
func TestShouldAddConfig(t *testing.T) {
	for _, args := range [][]string{
		{"site", "list"},
		{"site", "disable", "default"},
		{"config", "show", "--section", "acme"},
		{"cert", "list"},
		{"monitor", "status"},
		{"logs", "query", "--kind", "access"},
		{"diagnose"},
	} {
		if !shouldAddConfig(args) {
			t.Errorf("%v 应自动补 --config", args)
		}
	}
	for _, args := range [][]string{
		{"site", "list", "--config", "/x/fast-https.json"}, // 已显式带
		{"status"},
		{"version"},
		{"start"},
		{"stop"},
		{"reload"},
		{"config", "path"},
		{"acme", "show"},
		{"acme", "setup", "--non-interactive", "--email", "a@b.c"},
		{"service", "reload"},
		// 操作记录读状态目录，不接受 --config（实测 unknown flag，exit 2）
		{"operation", "list", "--limit", "8"},
		{"operation", "get", "op_20261110_001"},
		// 运行快照直连 127.0.0.1:9088，与配置文件无关（实测 unknown flag，exit 2）
		{"runtime", "status"},
		{"runtime", "stats"},
	} {
		if shouldAddConfig(args) {
			t.Errorf("%v 不应自动补 --config（该命令不接受此 flag）", args)
		}
	}
}
