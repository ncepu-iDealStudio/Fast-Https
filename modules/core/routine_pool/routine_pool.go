package routinepool

import (
	"fast-https/utils/message"
	"os"

	"github.com/panjf2000/ants/v2"
)

func panicHandler(err interface{}) {
	message.PrintWarn("--server routine panic", os.Stderr, err)
}

var (
	ServerPool *ants.Pool
)

func init() {
	ServerPool, _ = ants.NewPool(65535, ants.WithNonblocking(true),
		ants.WithPanicHandler(panicHandler))
}
