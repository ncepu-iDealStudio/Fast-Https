package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommandSpecsMatchCLITree 用 testdata/cli-commands.txt 锁定命令清单。
//
// 桌面端的定位是「CLI 有什么就图形化什么」，所以清单与 CLI 命令树必须一一
// 对应：漏一条，界面上就少一个能力；多一条，用户点了会得到 unknown command。
// CLI 加命令时，这张表也要跟着改，测试会立刻提醒。
func TestCommandSpecsMatchCLITree(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "cli-commands.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var want []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			want = append(want, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, s := range commandSpecs {
		if got[s.Name] {
			t.Fatalf("命令清单里 %q 重复了", s.Name)
		}
		got[s.Name] = true
		if s.Group == "" || s.Label == "" {
			t.Fatalf("%q 缺分组或标题", s.Name)
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("CLI 有 %q，但命令面板没列出来", name)
		}
	}
	for name := range got {
		found := false
		for _, w := range want {
			if w == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("命令面板列了 %q，但 CLI 命令树里没有", name)
		}
	}
	if len(commandSpecs) != len(want) {
		t.Fatalf("命令数 = %d，期望 %d", len(commandSpecs), len(want))
	}
}

// TestTextOnlyCommands 纯文本命令不能补 --json，否则 CLI 会当成未知 flag。
// status / acme show / config path 也是纯文本（前者旧命令无 JSON 分支，
// 后两者即使带 --json 也不套封套），命令面板必须走文本通道。
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

// TestCommandSpecsFlagsAreReal 锁定几个已废弃的假 flag：CLI 命令树里没有
// 的 flag 一旦混进清单，用户在面板里填了就会得到 unknown flag。
func TestCommandSpecsFlagsAreReal(t *testing.T) {
	bogus := map[string]bool{"--keyword": true, "--offset": true, "--id": true}
	for _, s := range commandSpecs {
		for _, f := range s.Flags {
			if bogus[f] {
				t.Errorf("%q 列了 CLI 里不存在的 flag %q", s.Name, f)
			}
		}
	}
	// 写操作必须标 NeedsYes，面板才会默认走 --dry-run 预演
	for _, name := range []string{"site add", "site update", "site delete", "config apply", "config rollback"} {
		found := false
		for _, s := range commandSpecs {
			if s.Name == name {
				found = true
				if !s.NeedsYes {
					t.Errorf("%q 应标 NeedsYes", name)
				}
			}
		}
		if !found {
			t.Errorf("清单缺少 %q", name)
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
