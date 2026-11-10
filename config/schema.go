// 配置文件的 JSON schema：文件键名与运行期结构在这里解耦，
// 解析出的 schema 由 convert.go 转成运行期配置。
package config

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"fast-https/infra/jsonc"
)

// unmarshalConfig 解析配置文件。格式是 infra/jsonc 支持的 JSONC：允许
// // 与 /* */ 注释、尾随逗号，解析结果与纯 JSON 完全一致。
func unmarshalConfig(data []byte, v any) error {
	return jsonc.Unmarshal(data, v)
}

// flexString 兼容 "listen": 8080（数字）与 "listen": "8080"（字符串）两种写法。
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*s = flexString(strconv.FormatFloat(f, 'f', -1, 64))
	return nil
}

// flexList 兼容 "listen": 8080、"listen": "8080" 与
// "listen": ["80", "443 ssl h2"]：一个 server 块可以同时监听多个端口。
// 加载阶段会把多监听展开成多个运行期 server 块（同 server_name、同 location、
// 同证书），端口监听与路由表因此不需要知道"一个块多个监听"这件事。
type flexList []string

func (l *flexList) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		list := make(flexList, 0, len(items))
		for _, item := range items {
			var one flexString
			if err := one.UnmarshalJSON(item); err != nil {
				return err
			}
			list = append(list, string(one))
		}
		*l = list
		return nil
	}
	var one flexString
	if err := one.UnmarshalJSON(trimmed); err != nil {
		return err
	}
	*l = flexList{string(one)}
	return nil
}

// proxyPassList 兼容 "proxy_pass": "http://10.0.0.1:8080" 与
// "proxy_pass": ["http://10.0.0.1:8080", "http://10.0.0.2:8080"]。
// 单个字符串的配置与以前完全一样。
type proxyPassList []string

func (l *proxyPassList) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*l = nil
		return nil
	}
	if trimmed[0] == '[' {
		var items []string
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		*l = proxyPassList(items)
		return nil
	}
	var one string
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return err
	}
	if strings.TrimSpace(one) == "" {
		*l = nil
		return nil
	}
	*l = proxyPassList{one}
	return nil
}

func (l proxyPassList) MarshalJSON() ([]byte, error) {
	if len(l) == 1 {
		return json.Marshal(l[0])
	}
	return json.Marshal([]string(l))
}

// First 返回唯一的上游字符串。多个地址时为空，避免把数组写成一个字符串。
func (l proxyPassList) First() string {
	if len(l) != 1 {
		return ""
	}
	return l[0]
}

func (l proxyPassList) schemeSample() string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

// MarshalJSON 单个监听按标量输出（纯端口写数字），既有配置与 site show 的
// 写法保持不变；多个监听才写成数组。
func (l flexList) MarshalJSON() ([]byte, error) {
	if len(l) == 1 {
		return json.Marshal(listenValue(l[0]))
	}
	values := make([]any, 0, len(l))
	for _, item := range l {
		values = append(values, listenValue(item))
	}
	return json.Marshal(values)
}

// listenValue 纯数字监听写成 JSON 数字，带 "ssl"/"h2" 时保持字符串。
func listenValue(listen string) any {
	if port, err := strconv.Atoi(strings.TrimSpace(listen)); err == nil {
		return port
	}
	return listen
}

type jsonPath struct {
	URL            string        `json:"url"`
	Type           string        `json:"type"`
	Root           string        `json:"root"`
	Index          []string      `json:"index"`
	Rewrite        string        `json:"rewrite"`
	ProxyPass      proxyPassList `json:"proxy_pass"`
	ProxySetHeader []Header      `json:"proxy_set_header"`
	AppFireWall    []string      `json:"appfirewall"`
	ProxyCache     Cache         `json:"proxy_cache"`
	Limit          PathLimit     `json:"limit"`
	Auth           PathAuth      `json:"auth"`
	ErrorPage      jsonErrorPage `json:"error_page"`
	Zip            []string      `json:"zip"`
	Try            []Try         `json:"try"`
	AllowMethods   []string      `json:"allow_methods"`
}

type jsonErrorPage struct {
	NotFound string `json:"not_found"`
	Proxy    string `json:"proxy"`
}

type jsonServer struct {
	Listen            flexList   `json:"listen"`
	ServerName        string     `json:"server_name"`
	SSLCertificate    string     `json:"ssl_certificate"`
	SSLCertificateKey string     `json:"ssl_certificate_key"`
	Locations         []jsonPath `json:"location"`
}

type jsonEngine struct {
	IsMaster     bool   `json:"is_master"`
	ID           int    `json:"id"`
	RegisterPort int    `json:"register_port"`
	SlaveIP      string `json:"slave_ip"`
	SlavePort    int    `json:"slave_port"`
}

type jsonHTTP struct {
	Include     []string     `json:"include"`
	DefaultType string       `json:"default_type"`
	Limit       ServerLimit  `json:"servers_limit"`
	BlackList   []string     `json:"blaklist"` // 历史拼写，配置兼容
	LogFormat   []string     `json:"log_format"`
	Servers     []jsonServer `json:"server"`
}

type jsonConfig struct {
	Pid      string          `json:"pid"`
	LogRoot  string          `json:"log_root"`
	Engine   jsonEngine      `json:"engine"`
	WAF      *WAFConfig      `json:"waf"`
	Acme     *AcmeConfig     `json:"acme"`
	Security *SecurityConfig `json:"security"`
	HTTP     jsonHTTP        `json:"http"`
}
