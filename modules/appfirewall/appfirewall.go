package appfirewall

import (
	"fast-https/modules/core/listener"
	"fast-https/modules/core/request"
	"fast-https/utils/logger"
)

var GAppFireWallMap map[string]func(*request.Request) bool

func init() {
	GAppFireWallMap = make(map[string]func(*request.Request) bool)
	GAppFireWallMap["sql"] = HandleSql
	GAppFireWallMap["xss"] = HandleXss
	logger.Info("appfirewall rule sql is registered and does not intercept requests")
}

// Blocks reports whether a named rule can reject a request.
// sql is registered for configuration compatibility and does not inspect the request.
func Blocks(name string) bool {
	return name == "xss"
}

func getReqInfo() {

}

// HandleAppFireWall runs configured rules.
// It returns false when a blocking rule rejects the request.
// Rules that do not intercept are skipped.
func HandleAppFireWall(cfg *listener.ListenCfg, req *request.Request) bool {
	allowed := true
	for _, val := range cfg.AppFireWall {
		if !Blocks(val) {
			logger.Debug("appfirewall rule %s does not intercept", val)
			continue
		}
		handler, ok := GAppFireWallMap[val]
		if !ok {
			logger.Debug("appfirewall rule %s does not intercept", val)
			continue
		}
		if !handler(req) {
			allowed = false
		}
	}
	return allowed
}
