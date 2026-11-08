package cmd

import (
	"errors"
	"fmt"
)

// 错误码与退出码一一对应，由 TestExitCodeTable 锁定。
// 规范的退出码表里没有 NOT_SUPPORTED，这里归入 11（依赖或环境不满足）。
const (
	CodeInternal         = "INTERNAL_ERROR"
	CodeUsage            = "USAGE_ERROR"
	CodePermissionDenied = "PERMISSION_DENIED"
	CodeServiceDown      = "SERVICE_UNREACHABLE"
	CodeConfigInvalid    = "CONFIG_INVALID"
	CodeOperationFailed  = "OPERATION_FAILED"
	CodeRecoveryRequired = "RECOVERY_REQUIRED"
	CodeOperationUnknown = "OPERATION_UNKNOWN"
	CodeTimeout          = "TIMEOUT"
	CodeNotFound         = "NOT_FOUND"
	CodeLogUnavailable   = "LOG_SOURCE_UNAVAILABLE"
	CodeConflict         = "CONFLICT"
	CodeRequestIDClash   = "REQUEST_ID_CONFLICT"
	CodeUnmetDependency  = "UNMET_DEPENDENCY"
	CodeNotSupported     = "NOT_SUPPORTED"
)

var exitCodes = map[string]int{
	CodeInternal:         1,
	CodeUsage:            2,
	CodePermissionDenied: 3,
	CodeServiceDown:      4,
	CodeConfigInvalid:    5,
	CodeOperationFailed:  6,
	CodeRecoveryRequired: 6,
	CodeOperationUnknown: 7,
	CodeTimeout:          8,
	CodeNotFound:         9,
	CodeLogUnavailable:   9,
	CodeConflict:         10,
	CodeRequestIDClash:   10,
	CodeUnmetDependency:  11,
	CodeNotSupported:     11,
}

// ErrorDetail 是错误封套 details 数组里的一项，例如一条校验错误。
type ErrorDetail struct {
	Code    string `json:"code,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// CLIError 是带稳定错误码的命令错误。只有它会走新的退出码映射，
// 旧命令返回的普通 error 一律是退出码 1，输出文本保持原样。
type CLIError struct {
	Code    string
	Message string
	Details []ErrorDetail
	// Command 是错误封套里的命令标识，例如 config.validate。
	// 留空时由入口按命令行参数推断。
	Command string
	Cause   error
}

func (e *CLIError) Error() string { return e.Message }

func (e *CLIError) Unwrap() error { return e.Cause }

// newCLIError 构造一个带错误码的错误。
func newCLIError(code, format string, args ...any) *CLIError {
	return &CLIError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// usageError 把 cobra 的参数或 flag 错误转成 USAGE_ERROR。
func usageError(err error) *CLIError {
	return &CLIError{Code: CodeUsage, Message: err.Error(), Cause: err}
}

// ExitCode 把错误映射成进程退出码。nil 是 0；不是 *CLIError 的错误和
// 不认识的错误码都是 1，不靠错误文本判断类型。
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ce *CLIError
	if errors.As(err, &ce) {
		if code, ok := exitCodes[ce.Code]; ok {
			return code
		}
	}
	return 1
}
