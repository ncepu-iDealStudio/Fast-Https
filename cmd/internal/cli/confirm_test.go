package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newConfirmCmd() *cobra.Command {
	root := &cobra.Command{Use: "fast-https"}
	sub := &cobra.Command{Use: "site update"}
	sub.Flags().Bool("yes", false, "")
	root.AddCommand(sub)
	return sub
}

// 非终端（这里是 strings.Reader）且没有 --yes：必须按用法错误拒绝。
func TestConfirmWriteRejectsNonTerminalWithoutYes(t *testing.T) {
	SetLang(LangEN)
	defer SetLang(LangZH)

	cmd := newConfirmCmd()
	cmd.SetIn(strings.NewReader("y\n"))

	err := ConfirmWrite(cmd, "update site web")
	var ce *CLIError
	if !errors.As(err, &ce) || ce.Code != CodeUsage {
		t.Fatalf("err = %v, want USAGE_ERROR", err)
	}
	if !strings.Contains(ce.Message, cmd.CommandPath()) || !strings.Contains(ce.Message, "--yes") {
		t.Fatalf("message = %q，应包含命令路径与 --yes", ce.Message)
	}
}

// --yes 在非终端下直接放行，不读取输入。
func TestConfirmWriteAcceptsYesFlag(t *testing.T) {
	cmd := newConfirmCmd()
	cmd.SetIn(strings.NewReader(""))
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmWrite(cmd, "update site web"); err != nil {
		t.Fatalf("带 --yes 应直接通过：%v", err)
	}
}

// 取消使用稳定错误码，退出码不再是 INTERNAL_ERROR 的语义。
func TestConfirmCancelledErrorCode(t *testing.T) {
	cmd := newConfirmCmd()
	err := cancelled(cmd)
	var ce *CLIError
	if !errors.As(err, &ce) || ce.Code != CodeCancelled {
		t.Fatalf("err = %v, want CANCELLED", err)
	}
	if ce.Command != cmd.CommandPath() {
		t.Fatalf("command = %q, want %q", ce.Command, cmd.CommandPath())
	}
	if got := ExitCode(err); got != 1 {
		t.Fatalf("exit = %d, want 1", got)
	}
}
