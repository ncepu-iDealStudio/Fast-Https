package appfirewall

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
)

var GAppFireWallMap map[string]func(*request.Request) bool

func init() {
	GAppFireWallMap = make(map[string]func(*request.Request) bool)
	GAppFireWallMap["sql"] = HandleSql
	GAppFireWallMap["xss"] = HandleXss
}

func getReqInfo() {

}

func HandleAppFireWall(cfg *listener.ListenCfg, req *request.Request) {
	for _, val := range cfg.AppFireWall {
		GAppFireWallMap[val](req)
	}
}
