package cmd

import "testing"

// acme 命令组迁到 internal/acmecmd 后，根命令树里仍然要能看到它。
// 原来这条断言在 acme_test.go 里，随包迁移后失去意义，改在这里守住。
func TestRootCmdRegistersAcme(t *testing.T) {
	for _, c := range RootCmd().Commands() {
		if c.Name() != "acme" {
			continue
		}
		subs := 0
		for range c.Commands() {
			subs++
		}
		if subs != 2 {
			t.Fatalf("acme subcommands = %d, want 2 (setup, show)", subs)
		}
		return
	}
	t.Fatal("acme 命令未注册到根命令树")
}
