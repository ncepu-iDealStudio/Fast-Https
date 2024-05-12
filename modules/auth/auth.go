package auth

// 配置文件 用户请求
import (
	"encoding/base64"
	"fast-https/modules/core"
	"fast-https/modules/core/listener"
	"fast-https/modules/core/response"
	"fast-https/utils/message"
	"strings"
)

// 在这里传参进入简单的用户名和密码，实际使用中应该使用更安全的方式存储和验证

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
		// fmt.Println(result)
		decodedBytes, err := base64.StdEncoding.DecodeString(result[1])

		if err != nil {
			message.PrintWarn(err.Error())
		}
		// 将解码后的字节切片转换为字符串
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
	//fmt.Printf("%p", ev)
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
