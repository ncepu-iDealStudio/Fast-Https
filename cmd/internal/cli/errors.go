package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"fast-https/config"
	"fast-https/management/operation"
	"fast-https/management/state"
	"fast-https/management/svcctl"
	"fast-https/management/txn"
	"fast-https/management/view"
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

// ExitCodes 返回错误码到退出码的映射副本。退出码是对外契约，
// 测试与文档从这里取，改表时能立刻被发现。
func ExitCodes() map[string]int {
	out := make(map[string]int, len(exitCodes))
	for k, v := range exitCodes {
		out[k] = v
	}
	return out
}

// ErrorDetail 是错误封套 details 数组里的一项，例如一条校验错误。
type ErrorDetail struct {
	Code    string `json:"code,omitempty"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// CLIError 是带稳定错误码的命令错误。只有它会走退出码映射，
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

// New 构造一个带错误码的错误。
func New(code, format string, args ...any) *CLIError {
	return &CLIError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// UsageError 把 cobra 的参数或 flag 错误转成 USAGE_ERROR。
func UsageError(err error) *CLIError {
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

// FromTxnError 把配置事务的错误转成带错误码的 CLIError。
func FromTxnError(id string, err error) *CLIError {
	if err == nil {
		return nil
	}
	ce := &CLIError{Command: id, Message: err.Error(), Cause: err}
	var te *txn.Error
	if errors.As(err, &te) {
		ce.Code = mapTxnCode(te.Code)
		ce.Message = te.Error()
		if te.Operation != nil {
			ce.Details = append(ce.Details, ErrorDetail{Path: "operation_id", Message: te.Operation.ID})
			if te.Operation.Result != nil {
				if p, ok := te.Operation.Result["restore_point"].(string); ok && p != "" {
					ce.Details = append(ce.Details, ErrorDetail{Path: "restore_point", Message: p})
				}
				if s, ok := te.Operation.Result["config_state"].(string); ok && s != "" {
					ce.Details = append(ce.Details, ErrorDetail{Path: "config_state", Message: s})
				}
			}
		}
		var pe *svcctl.PortError
		if errors.As(te.Err, &pe) {
			ce.Details = append(ce.Details, ErrorDetail{Code: txn.CodePortInUse, Path: "port", Message: pe.Error()})
		}
		return ce
	}
	var pe *svcctl.PortError
	if errors.As(err, &pe) {
		ce.Code = CodeConflict
		ce.Details = []ErrorDetail{{Code: txn.CodePortInUse, Path: "port", Message: pe.Error()}}
		return ce
	}
	switch {
	case errors.Is(err, config.ErrCandidatePath):
		ce.Code = CodeUsage
	case errors.Is(err, operation.ErrRequestIDConflict):
		ce.Code = CodeRequestIDClash
	case errors.Is(err, operation.ErrNotFound):
		ce.Code = CodeNotFound
	case errors.Is(err, state.ErrLocked):
		ce.Code = CodeConflict
	case errors.Is(err, svcctl.ErrNotRunning):
		ce.Code = CodeServiceDown
	case errors.Is(err, svcctl.ErrTimeout):
		ce.Code = CodeTimeout
	case errors.Is(err, svcctl.ErrReloadNotApplied), errors.Is(err, svcctl.ErrReloadedAsRestart):
		ce.Code = CodeOperationFailed
	case errors.Is(err, os.ErrPermission):
		ce.Code = CodePermissionDenied
	default:
		ce.Code = CodeInternal
	}
	return ce
}

func mapTxnCode(code string) string {
	switch code {
	case txn.CodePermission:
		return CodePermissionDenied
	case txn.CodeConfigInvalid:
		return CodeConfigInvalid
	case txn.CodeConflict, txn.CodePortInUse:
		return CodeConflict
	case txn.CodeFailed:
		return CodeOperationFailed
	case txn.CodeRecovery:
		return CodeRecoveryRequired
	case txn.CodeUnknown:
		return CodeOperationUnknown
	case txn.CodeTimeout:
		return CodeTimeout
	case txn.CodeNotFound:
		return CodeNotFound
	case txn.CodeUsage:
		return CodeUsage
	case txn.CodeRequestClash:
		return CodeRequestIDClash
	case txn.CodeServiceDown:
		return CodeServiceDown
	default:
		return CodeInternal
	}
}

// FromStateError 把操作记录与状态目录的错误转成带错误码的 CLIError。
func FromStateError(id string, err error) *CLIError {
	switch {
	case errors.Is(err, operation.ErrNotFound):
		return &CLIError{Code: CodeNotFound, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, operation.ErrRequestIDConflict):
		return &CLIError{Code: CodeRequestIDClash, Command: id, Message: err.Error(), Cause: err}
	case errors.Is(err, state.ErrLocked):
		return &CLIError{Code: CodeConflict, Command: id, Message: err.Error(), Cause: err}
	default:
		return &CLIError{Code: CodeInternal, Command: id, Message: err.Error(), Cause: err}
	}
}

// FromViewError 把视图层返回的错误转成带错误码的 CLIError。
// 读配置时其它的解析或读取错误一律按配置问题（CONFIG_INVALID）处理。
func FromViewError(id string, err error) *CLIError {
	if err == nil {
		return nil
	}
	ce := &CLIError{Command: id, Message: err.Error(), Cause: err}

	var invalid *view.InvalidError
	switch {
	case errors.As(err, &invalid):
		ce.Code = CodeConfigInvalid
		ce.Details = []ErrorDetail{{Code: CodeConfigInvalid, Message: invalid.Message}}
	case view.IsNotFound(err):
		ce.Code = CodeNotFound
	case errors.Is(err, view.ErrBadSection), errors.Is(err, view.ErrBadName), errors.Is(err, config.ErrCandidatePath):
		ce.Code = CodeUsage
	case errors.Is(err, fs.ErrPermission):
		ce.Code = CodePermissionDenied
	default:
		ce.Code = CodeConfigInvalid
		ce.Details = []ErrorDetail{{Code: CodeConfigInvalid, Message: err.Error()}}
	}
	return ce
}
