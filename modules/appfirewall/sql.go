package appfirewall

import (
	"fast-https/modules/core/request"
	"fmt"
)

func HandleSql(req *request.Request) bool {
	fmt.Println("This is appfirewall, handle sql")

	return true
}
