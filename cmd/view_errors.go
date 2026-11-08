package cmd

import (
	"errors"
	"io/fs"

	"fast-https/management/view"
)

// fromViewError 把视图层返回的错误转成带错误码的 CLIError。
// 读配置时其它的解析或读取错误一律按配置问题（CONFIG_INVALID）处理。
func fromViewError(cmdID string, err error) *CLIError {
	if err == nil {
		return nil
	}
	ce := &CLIError{Command: cmdID, Message: err.Error(), Cause: err}

	var invalid *view.InvalidError
	switch {
	case errors.As(err, &invalid):
		ce.Code = CodeConfigInvalid
		ce.Details = []ErrorDetail{{Code: CodeConfigInvalid, Message: invalid.Message}}
	case view.IsNotFound(err):
		ce.Code = CodeNotFound
	case errors.Is(err, view.ErrBadSection), errors.Is(err, view.ErrBadName):
		ce.Code = CodeUsage
	case errors.Is(err, fs.ErrPermission):
		ce.Code = CodePermissionDenied
	default:
		ce.Code = CodeConfigInvalid
		ce.Details = []ErrorDetail{{Code: CodeConfigInvalid, Message: err.Error()}}
	}
	return ce
}
