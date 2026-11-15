package txn

import (
	"errors"
	"fmt"

	"fast-https/management/operation"
	"fast-https/management/svcctl"
)

const (
	CodePermission    = "PERMISSION_DENIED"
	CodeConfigInvalid = "CONFIG_INVALID"
	CodeConflict      = "CONFLICT"
	CodePortInUse     = "PORT_IN_USE"
	CodeFailed        = "OPERATION_FAILED"
	CodeRecovery      = "RECOVERY_REQUIRED"
	CodeUnknown       = "OPERATION_UNKNOWN"
	CodeTimeout       = "TIMEOUT"
	CodeNotFound      = "NOT_FOUND"
	CodeUsage         = "USAGE_ERROR"
	CodeRequestClash  = "REQUEST_ID_CONFLICT"
	CodeServiceDown   = "SERVICE_UNREACHABLE"
)

// Error 是事务失败。CLI 按 Code 映射退出码。
type Error struct {
	Code      string
	Message   string
	Operation *operation.Record
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func wrap(code, msg string, err error) *Error {
	return &Error{Code: code, Message: msg, Err: err}
}

func asTxn(err error) *Error {
	var te *Error
	if errors.As(err, &te) {
		return te
	}
	var pe *svcctl.PortError
	if errors.As(err, &pe) {
		return &Error{Code: CodeConflict, Message: pe.Error(), Err: err}
	}
	if errors.Is(err, operation.ErrRequestIDConflict) {
		return wrap(CodeRequestClash, err.Error(), err)
	}
	if errors.Is(err, operation.ErrNotFound) {
		return wrap(CodeNotFound, err.Error(), err)
	}
	if errors.Is(err, svcctl.ErrNotRunning) {
		return wrap(CodeServiceDown, err.Error(), err)
	}
	if errors.Is(err, svcctl.ErrTimeout) {
		return wrap(CodeTimeout, err.Error(), err)
	}
	return wrap(CodeFailed, err.Error(), err)
}

func attachOp(err error, rec operation.Record) error {
	te := asTxn(err)
	cp := rec
	te.Operation = &cp
	if te.Message == "" {
		te.Message = fmt.Sprintf("%s (operation %s)", te.Code, rec.ID)
	}
	return te
}
