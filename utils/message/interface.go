package message

import (
	"fmt"
)

type message struct {
	Context any
	Type    string
}

func send(msg message) {
	if !rwMutex.TryRLock() {
		return
	}
	defer rwMutex.RUnlock()
	if outputChan == nil {
		return
	}
	outputChan.In <- msg
}

func PrintInfo(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "info",
	})
}

func PrintWarn(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "warn",
	})
}

func PrintErr(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "err",
	})
}

func Printf(format string, a ...interface{}) {
	send(message{
		Context: fmt.Sprintf(format, a...),
		Type:    "msg",
	})
}

func Exit() {
	send(message{
		Context: "",
		Type:    "exit",
	})
}

func PrintRecover(a any) {
	send(message{
		Context: a,
		Type:    "recover",
	})
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
	send(message{
		Context: context,
		Type:    "access",
	})
}

func PrintSafe(a ...interface{}) {
	send(message{
		Context: fmt.Sprint(a...),
		Type:    "safe",
	})
}
