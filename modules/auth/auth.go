package auth

import (
	"encoding/base64"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"strings"
)

func AuthHandler(cfg *listener.ListenCfg, ev *core.Event) bool {
	if !AuthExists(cfg) {
		return true
	}
	req := ev.RR.Req
	basic := req.GetAuthorization()
	var username string
	var pswd string
	if basic != "" {
		result := strings.Fields(basic)
		decodedBytes, err := base64.StdEncoding.DecodeString(result[1])

		if err != nil {
			message.PrintWarn(err.Error())
		}
		decodedStr := string(decodedBytes)

		username = strings.Split(decodedStr, ":")[0]
		pswd = strings.Split(decodedStr, ":")[1]
	}

	if username == cfg.Auth.User && pswd == cfg.Auth.Pswd {
		return true
	}
	res := response.ResponseInit()
	res.SetFirstLine(401, "Authorization Required")
	res.SetHeader("www-Authenticate", "Basic realm=\"Access to the staging site\"")
	ev.RR.Res = res
	ev.WriteResponseClose(nil)
	return false
}

func AuthExists(cfg *listener.ListenCfg) bool {
	if cfg.Auth.AuthType != "" {
		return true
	} else {
		return false
	}
}
