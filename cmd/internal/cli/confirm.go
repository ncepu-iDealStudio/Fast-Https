package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// ConfirmWrite 在非 dry-run 的写操作前做确认：
//   - 带 --yes：直接通过；
//   - 标准输入是终端：打印提示并读取 y/N，适合人工在终端里操作；
//   - 非终端（脚本、桌面端、CI）：仍要求显式 --yes，避免静默写入。
//
// 终端判定与读取都用 cmd.InOrStdin()，避免 SetIn 注入输入时判定与读取源不一致。
// action 是给用户看的动作描述，例如「更新站点 web」/「delete cache zone edge」。
func ConfirmWrite(cmd *cobra.Command, action string) error {
	if Confirmed(cmd) {
		return nil
	}
	in := cmd.InOrStdin()
	file, ok := in.(*os.File)
	if !ok || !term.IsTerminal(file.Fd()) {
		return UsageError(fmt.Errorf(T("%s 需要显式 --yes；可先用 --dry-run 预览",
			"%s requires --yes; use --dry-run to preview"), cmd.CommandPath()))
	}
	fmt.Fprintf(cmd.ErrOrStderr(), T("即将%s。继续？[y/N] ", "Proceed with %s? [y/N] "), action)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return cancelled(cmd)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return cancelled(cmd)
	}
}

// cancelled 把用户取消映射成稳定错误码，避免 --json 下被当成 INTERNAL_ERROR。
func cancelled(cmd *cobra.Command) error {
	return &CLIError{Code: CodeCancelled, Command: cmd.CommandPath(), Message: T("已取消", "cancelled")}
}
