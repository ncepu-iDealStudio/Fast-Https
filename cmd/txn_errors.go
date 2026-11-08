package cmd

import (
	"errors"
	"os"

	"fast-https/config"
	"fast-https/management/operation"
	"fast-https/management/state"
	"fast-https/management/svcctl"
	"fast-https/management/txn"
)

func fromTxnError(id string, err error) *CLIError {
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
