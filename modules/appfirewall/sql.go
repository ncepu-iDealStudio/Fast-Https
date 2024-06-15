package appfirewall

import (
	"fast-https/modules/core/request"
	"fast-https/utils/logger"
)

func HandleSql(req *request.Request) bool {
	logger.Debug("This is appfirewall, handle sql")

	return true
}
