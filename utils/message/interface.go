package message

import (
	"fmt"
)

type message struct {
	Context any
	Type    string
}

func PrintInfo(a ...interface{}) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: fmt.Sprint(a...),
			Type:    "info",
		}
	}
}

func PrintWarn(a ...interface{}) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: fmt.Sprint(a...),
			Type:    "warn",
		}
	}
}

func PrintErr(a ...interface{}) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: fmt.Sprint(a...),
			Type:    "err",
		}
	}
}

func Printf(format string, a ...interface{}) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: fmt.Sprintf(format, a...),
			Type:    "msg",
		}
	}
}

func Exit() {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: "",
			Type:    "exit",
		}
	}
}

func PrintRecover(a any) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: a,
			Type:    "recover",
		}
	}
}

// PrintAccess
//
//	@Description: print access log
//	@param host: access host
//	@param a: any other log message
func PrintAccess(host string, a ...interface{}) {
	context := map[string]any{
		"host":    host,
		"message": a,
	}
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: context,
			Type:    "access",
		}
	}
}

func PrintSafe(a ...interface{}) {
	if rwMutex.TryRLock() {
		defer rwMutex.RUnlock()
		outputChan.In <- message{
			Context: fmt.Sprint(a...),
			Type:    "safe",
		}
	}
}
