// Package schema 是配置文件的 JSON 结构：文件键名与运行期结构在这里解耦，
// 解析结果由 loader 转成运行期配置。listen 的 flex 类型也在这里，
// 让主配置与站点文件对 listen 的写法保持一致。
package schema

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"fast-https/config/internal/model"
	"fast-https/infra/jsonc"
)

// Unmarshal 解析配置文件。格式是 infra/jsonc 支持的 JSONC：允许
// // 与 /* */ 注释、尾随逗号，解析结果与纯 JSON 完全一致。
func Unmarshal(data []byte, v any) error {
	return jsonc.Unmarshal(data, v)
}

// FlexString 兼容 "listen": 8080（数字）与 "listen": "8080"（字符串）两种写法。
type FlexString string

func (s *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = FlexString(v)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*s = FlexString(strconv.FormatFloat(f, 'f', -1, 64))
	return nil
}

// FlexList 兼容 "listen": 8080、"listen": "8080" 与
// "listen": ["80", "443 ssl h2"]：一个 server 块可以同时监听多个端口。
// 加载阶段会把多监听展开成多个运行期 server 块（同 server_name、同 location、
// 同证书），端口监听与路由表因此不需要知道"一个块多个监听"这件事。
type FlexList []string

func (l *FlexList) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return err
		}
		list := make(FlexList, 0, len(items))
		for _, item := range items {
			var one FlexString
			if err := one.UnmarshalJSON(item); err != nil {
				return err
			}
			list = append(list, string(one))
		}
		*l = list
		return nil
	}
	var one FlexString
	if err := one.UnmarshalJSON(trimmed); err != nil {
		return err
	}
	*l = FlexList{string(one)}
	return nil
}

// MarshalJSON 单个监听按标量输出（纯端口写数字），既有配置与 site show 的
// 写法保持不变；多个监听才写成数组。
func (l FlexList) MarshalJSON() ([]byte, error) {
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

// ListenPort 从 listen 写法里取端口号，取不到返回 0。
// 写法形如 "80"、"443 ssl h2"、":8080"，端口必须是第一段。
func ListenPort(listen string) int {
	fields := strings.Fields(strings.TrimSpace(listen))
	if len(fields) == 0 {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimPrefix(fields[0], ":"))
	if err != nil {
		return 0
	}
	return port
}

type Path struct {
	URL            string          `json:"url"`
	Type           string          `json:"type"`
	Root           string          `json:"root"`
	SymlinkPolicy  string          `json:"symlink_policy,omitempty"`
	Index          []string        `json:"index"`
	Rewrite        string          `json:"rewrite"`
	ProxyPass      string          `json:"proxy_pass"`
	ProxySetHeader []model.Header  `json:"proxy_set_header"`
	AppFireWall    []string        `json:"appfirewall"`
	ProxyCache     model.Cache     `json:"proxy_cache"`
	Limit          model.PathLimit `json:"limit"`
	Auth           model.PathAuth  `json:"auth"`
	ErrorPage      ErrorPage       `json:"error_page"`
	Zip            []string        `json:"zip"`
	// ZipMinLength 是压缩阈值（字节），0 表示不限制；小于该值的文件不压缩。
	ZipMinLength int `json:"zip_min_length"`
	// ZipSkipTypes 是跳过压缩的 MIME 类型；留空使用内置的已压缩类型清单，非空整体替换。
	ZipSkipTypes []string    `json:"zip_skip_types"`
	Try          []model.Try `json:"try"`
	AllowMethods []string    `json:"allow_methods"`
}

type ErrorPage struct {
	NotFound string `json:"not_found"`
	Proxy    string `json:"proxy"`
}

type Server struct {
	Listen            FlexList `json:"listen"`
	ServerName        string   `json:"server_name"`
	SSLCertificate    string   `json:"ssl_certificate"`
	SSLCertificateKey string   `json:"ssl_certificate_key"`
	Locations         []Path   `json:"location"`
}

type Engine struct {
	IsMaster     bool   `json:"is_master"`
	ID           int    `json:"id"`
	RegisterPort int    `json:"register_port"`
	SlaveIP      string `json:"slave_ip"`
	SlavePort    int    `json:"slave_port"`
}

type HTTP struct {
	Include     []string          `json:"include"`
	DefaultType string            `json:"default_type"`
	Limit       model.ServerLimit `json:"servers_limit"`
	BlackList   []string          `json:"blaklist"` // 历史拼写，配置兼容
	LogFormat   []string          `json:"log_format"`
	Servers     []Server          `json:"server"`
}

type Config struct {
	Pid      string                `json:"pid"`
	LogRoot  string                `json:"log_root"`
	Engine   Engine                `json:"engine"`
	WAF      *model.WAFConfig      `json:"waf"`
	Acme     *model.AcmeConfig     `json:"acme"`
	Security *model.SecurityConfig `json:"security"`
	TLS      TLS                   `json:"tls"`
	Proxy    Proxy                 `json:"proxy"`
	HTTP     HTTP                  `json:"http"`
}

// Proxy 是全局代理策略段：可信代理、X-Forwarded-For 链长上限与 Host 透传。
type Proxy struct {
	TrustedProxies []string     `json:"trusted_proxies"`
	ForwardedFor   ForwardedFor `json:"forwarded_for"`
	PreserveHost   *bool        `json:"preserve_host"`
}

// ForwardedFor 是 X-Forwarded-For 链的配置。max_hops 为 0 表示不限制。
type ForwardedFor struct {
	MaxHops int `json:"max_hops"`
}

// TLS 是全局 TLS 段。default_certificate 只在 SNI 未命中任何站点证书时
// 兜底，不改变按域名选证书的行为。
type TLS struct {
	DefaultCertificate Certificate `json:"default_certificate"`
}

// Certificate 是一对证书文件路径。
type Certificate struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}
