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

// syncCalculateSum := func() {
// 	// if each_event.LisInfo.LisType == 10 {
// 	// events.H2HandleEvent(l, conn, &(s.Shutdown))
// 	// } else {
// 	events.HandleEvent(l, conn, &(s.Shutdown))
// 	// }
// 	s.wg.Done()
// }

// submitErr := routinepool.ServerPool.Submit(syncCalculateSum)
// if submitErr != nil {
// 	message.PrintWarn("--server: Submit events:", submitErr)
// 	// there is no more routine to handle this request...
// 	// just close it
// 	conn.Close()
// } else {
// 	s.wg.Add(1)
// }
