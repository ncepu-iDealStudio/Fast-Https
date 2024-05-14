package appfirewall

import (
	"fast-https/modules/core/listener"
)

var GAppFireWallMap map[string]func(string) bool

func init() {
	GAppFireWallMap = make(map[string]func(string) bool)
	GAppFireWallMap["sql"] = HandleSql
	GAppFireWallMap["xss"] = HandleXss
}

func getReqInfo() {

}

func HandleAppFireWall(cfg *listener.ListenCfg) {
	for _, val := range cfg.AppFireWall {
		GAppFireWallMap[val]("test")
	}
}
