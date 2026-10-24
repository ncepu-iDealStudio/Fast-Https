// Package auth 校验按 location 配置的 HTTP Basic 认证凭据。
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"fast-https/core"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"
	"fast-https/infra/logging"
	"strings"
)

func AuthHandler(cfg *listener.ListenCfg, ev *core.Event) bool {
	if !AuthExists(cfg) {
		return true
	}
	username, pswd, ok := parseBasicAuth(ev.RR.Req.GetAuthorization())
	if !ok || !credentialsMatch(cfg, username, pswd) {
		unauthorized(ev)
		return false
	}
	return true
}

// parseBasicAuth 解析 "Basic base64(user:pswd)"。
// 任何格式问题都返回 ok=false：此前对畸形 Authorization 头直接取
// result[1] 或 Split(":")[1] 会越界 panic，单个请求即可打死进程。
func parseBasicAuth(basic string) (string, string, bool) {
	if basic == "" {
		return "", "", false
	}
	fields := strings.Fields(basic)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Basic") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		logging.PrintWarn(err.Error())
		return "", "", false
	}
	i := strings.Index(string(decoded), ":")
	if i < 0 {
		return "", "", false
	}
	return string(decoded[:i]), string(decoded[i+1:]), true
}

// credentialsMatch 用常数时间比较，避免用户名/密码的时序侧信道。
func credentialsMatch(cfg *listener.ListenCfg, username, pswd string) bool {
	return subtle.ConstantTimeCompare([]byte(username), []byte(cfg.Auth.User)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pswd), []byte(cfg.Auth.Pswd)) == 1
}

func unauthorized(ev *core.Event) {
	res := response.ResponseInit()
	res.SetFirstLine(401, "Authorization Required")
	res.SetHeader("WWW-Authenticate", "Basic realm=\"Access to the staging site\"")
	res.SetHeader("Content-Length", "0")
	ev.RR.Res = res
	ev.WriteResponseClose(nil)
}

func AuthExists(cfg *listener.ListenCfg) bool {
	return cfg.Auth.AuthType != ""
}
