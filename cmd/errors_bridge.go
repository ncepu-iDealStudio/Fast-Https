package cmd

import "fast-https/cmd/internal/cli"

// 错误与退出码的实现已迁到 cmd/internal/cli。这里保留 cmd 包内的旧名字，
// 让几十个调用点不必与迁移同步改动；迁移收尾时整体删除。

// CLIError 是带稳定错误码的命令错误（别名，实现在 internal/cli）。
type CLIError = cli.CLIError

// ErrorDetail 是错误封套 details 数组里的一项。
type ErrorDetail = cli.ErrorDetail

const (
	CodeInternal         = cli.CodeInternal
	CodeUsage            = cli.CodeUsage
	CodePermissionDenied = cli.CodePermissionDenied
	CodeServiceDown      = cli.CodeServiceDown
	CodeConfigInvalid    = cli.CodeConfigInvalid
	CodeOperationFailed  = cli.CodeOperationFailed
	CodeRecoveryRequired = cli.CodeRecoveryRequired
	CodeOperationUnknown = cli.CodeOperationUnknown
	CodeTimeout          = cli.CodeTimeout
	CodeNotFound         = cli.CodeNotFound
	CodeLogUnavailable   = cli.CodeLogUnavailable
	CodeConflict         = cli.CodeConflict
	CodeRequestIDClash   = cli.CodeRequestIDClash
	CodeUnmetDependency  = cli.CodeUnmetDependency
	CodeNotSupported     = cli.CodeNotSupported
)

func newCLIError(code, format string, args ...any) *CLIError {
	return cli.New(code, format, args...)
}

func usageError(err error) *CLIError { return cli.UsageError(err) }

// ExitCode 把错误映射成进程退出码。
func ExitCode(err error) int { return cli.ExitCode(err) }

func fromTxnError(id string, err error) *CLIError { return cli.FromTxnError(id, err) }

func fromViewError(id string, err error) *CLIError { return cli.FromViewError(id, err) }
