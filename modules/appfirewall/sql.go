package appfirewall

import (
	"fast-https/runtime/request"
	"fast-https/infra/logging"
)

func HandleSql(req *request.Request) bool {
	logging.ConsoleLog.Debug().Msg("This is appfirewall, handle sql")

	return true
}
