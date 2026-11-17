// Package model 定义运行期配置模型：类型、常量、编译期路径基址与配置快照。
// 本包不读文件、不校验，只被 config 内部其他包与外部读取方使用。
package model

import (
	"net/netip"
	"strings"
)

const (
	ZIP_NONE    = 0
	ZIP_GZIP    = 1
	ZIP_BR      = 2
	ZIP_GZIP_BR = 10

	Host        = 100
	XRealIp     = 101
	XForwardFor = 102
)

/*
// path type
*/
const (
	LOCAL       = 0
	PROXY_HTTP  = 1
	PROXY_HTTPS = 2
	PROXY_TCP   = 3
	REWRITE     = 4

	DEVMOD = 10
)

// ---------- 配置类型 ----------
// json 标签即配置文件键名；历史拼写（如 http.blaklist）原样保留以兼容存量配置。

type Header struct {
	HeaderKey   string `json:"HeaderKey"`
	HeaderValue string `json:"HeaderValue"`
}

type Cache struct {
	Path    string   `json:"path"`
	Valid   []string `json:"valid"`
	Key     string   `json:"key"`
	Methods []string `json:"methods"` // 允许读写的请求方法，留空等价于 GET 与 HEAD
	MaxSize int      `json:"max_size"`
}

// IsZero 报告这个缓存配置是否等于「未配置」：没有 path 就没有缓存目录，
// 此时删掉 proxy_cache 键。
func (c Cache) IsZero() bool {
	return strings.TrimSpace(c.Path) == "" && len(c.Valid) == 0 &&
		strings.TrimSpace(c.Key) == "" && len(c.Methods) == 0 && c.MaxSize == 0
}

type PathAuth struct {
	AuthType string `json:"type"`
	User     string `json:"user"`
	Pswd     string `json:"pswd"`
	// Realm 是 401 响应里 WWW-Authenticate 的 realm，留空时用站点名。
	Realm string `json:"realm"`
}

// IsZero 报告这个 auth 是否等于「未配置」：四个字段全空。
// 全空时 site update 删掉 auth 键，等于关闭该 location 的 Basic 认证。
func (a PathAuth) IsZero() bool {
	return a == PathAuth{}
}

type PathLimit struct {
	Size    int  `json:"mem"`
	Rate    int  `json:"rate"`
	Burst   int  `json:"burst"`
	Nodelay bool `json:"nodelay"`
}

// IsZero 报告这个 limit 是否等于「未配置」。全零时 site update 会删掉
// limit 键而不是写一个 `"limit": {}`——后者会被校验当成配了限流但全是 0。
func (l PathLimit) IsZero() bool {
	return l == PathLimit{}
}

type ServerLimit struct {
	MaxBodySize   int `json:"max_body_size"`
	MaxHeaderSize int `json:"max_header_size"`
	Rate          int `json:"limit"`
	Burst         int `json:"burst"`
}

type Try struct {
	Uri   string   `json:"uri"`
	Files []string `json:"file"`
	Next  string   `json:"next"`
}

type Path struct {
	PathName string
	PathType uint16
	Zip      uint16
	Root     string
	// SymlinkPolicy 控制站点根下符号链接的处理方式，取值见 SYMLINK_* 常量。
	SymlinkPolicy  string
	Index          []string
	Rewrite        string
	Trys           []Try
	ProxyData      string
	ProxySetHeader []Header
	AppFireWall    []string
	ProxyCache     Cache
	Limit          PathLimit
	Auth           PathAuth
	ErrorNotFound  string
	ErrorProxy     string
	AllowMethods   []string
}

type Server struct {
	Listen string

	ServerName        string
	SSLCertificate    string
	SSLCertificateKey string
	Path              []Path
}

// ServerNames 把 server_name 按空白分割成域名列表（nginx 风格多域名写法：
// "a.example b.example"）。单域名配置返回单元素切片，行为与之前一致。
// 返回的每个域名已 TrimSpace，空段被丢弃。
func (s Server) ServerNames() []string {
	return strings.Fields(s.ServerName)
}

// PrimaryServerName 返回第一个域名（主域名），用于证书文件名、ACME 申请、
// 日志展示等需要单值的场景。空 server_name 返回空串。
func (s Server) PrimaryServerName() string {
	names := s.ServerNames()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

type Engine struct {
	IsMaster     bool
	Id           int
	RegisterPort int    // master uses, default 9099
	SlaveIp      string // slave uses
	SlavePort    int    // slave uses
}

// WAFConfig 是全局应用防火墙（GuardianWAF）的最小配置。
// 不配置 waf 块且无任何 location 配置 appfirewall 时不构建引擎（零开销）。
type WAFConfig struct {
	// Mode 引擎模式：enforce（默认，命中阻断）、monitor（只记录不阻断）、
	// disabled（等价于不启用）。
	Mode string `json:"mode"`
}

// Fast_Https 是运行期配置快照。schema 里的预留字段（pid、engine）保留以
// 兼容存量配置；nginx 风格的调优字段（autoindex、sendfile、tcp_nopush 等）
// 从未被任何模块读取，已删除。
type Fast_Https struct {
	Pid     string
	LogRoot string

	Servers      []Server
	ServerEngine Engine
	Limit        ServerLimit
	BlackList    []string
	LogFormat    []string
	Include      []string
	DefaultType  string
	Waf          *WAFConfig
	Acme         *AcmeConfig
	Security     *SecurityConfig
	TLS          TLSConfig
	Proxy        Proxy
}

// ForwardedForConfig 是 proxy.forwarded_for 段的原始配置。
type ForwardedForConfig struct {
	MaxHops int `json:"max_hops"`
}

// ProxyConfig 是顶层 proxy 段的原始写法，供命令行读写使用。
// PreserveHost 用指针区分「没写」（默认 true）与显式 false。
type ProxyConfig struct {
	TrustedProxies []string           `json:"trusted_proxies"`
	ForwardedFor   ForwardedForConfig `json:"forwarded_for"`
	PreserveHost   *bool              `json:"preserve_host"`
}

// PreserveHostEnabled 报告是否透传客户端 Host。未配置时为 true。
func (p ProxyConfig) PreserveHostEnabled() bool {
	return p.PreserveHost == nil || *p.PreserveHost
}

// Proxy 是运行期代理策略。TrustedPrefixes 是 TrustedProxies 编译后的
// 网段列表，只在进程内使用，不参与 JSON 序列化。
type Proxy struct {
	TrustedProxies      []string
	ForwardedForMaxHops int
	PreserveHost        bool
	TrustedPrefixes     []netip.Prefix
}

// TLSConfig 是全局 TLS 段。DefaultCertificate 只在 SNI 未命中任何站点
// 证书时兜底；未配置时沿用历史行为（回落证书列表第一张）。
type TLSConfig struct {
	DefaultCertificate Certificate `json:"default_certificate"`
}

// Certificate 是一对证书文件路径。
type Certificate struct {
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

// HasDefaultCertificate 报告是否配置了兜底证书。
func (t TLSConfig) HasDefaultCertificate() bool {
	return strings.TrimSpace(t.DefaultCertificate.Certificate) != "" &&
		strings.TrimSpace(t.DefaultCertificate.Key) != ""
}

// Path 的符号链接策略取值。默认 check-at-load：加载/重载时站点根下出现
// 符号链接即拒绝；reject 在请求期把逃出站点根的链接按未找到处理；
// allow 保持历史行为。
const (
	SYMLINK_ALLOW         = "allow"
	SYMLINK_REJECT        = "reject"
	SYMLINK_CHECK_AT_LOAD = "check-at-load"
)

// NormalizeSymlinkPolicy 统一符号链接策略写法：空值按默认 check-at-load。
func NormalizeSymlinkPolicy(policy string) string {
	policy = strings.ToLower(strings.TrimSpace(policy))
	if policy == "" {
		return SYMLINK_CHECK_AT_LOAD
	}
	return policy
}

// ACMECA 是降级链中的一个 CA。ZeroSSL 这类 CA 要求账户绑定 EAB，
// EAB 缺失时该 CA 会被跳过，不做匿名注册尝试。
type ACMECA struct {
	Name       string `json:"name"`
	Directory  string `json:"directory"`
	EABKid     string `json:"eab_kid,omitempty"`
	EABHmacKey string `json:"eab_hmac_key,omitempty"`
}

// AcmeConfig 是顶层 acme 段。缺省时 enabled=false。
type AcmeConfig struct {
	Enabled   bool     `json:"enabled"`
	Email     string   `json:"email"`
	Staging   bool     `json:"staging"`
	Challenge []string `json:"challenge"`
	// RenewBeforeDays 证书到期前的续期提前量，0 表示用默认 30 天。
	RenewBeforeDays int `json:"renew_before_days"`
	// FallbackSelfSigned 用指针区分「没写」与「显式 false」：默认回落自签。
	FallbackSelfSigned *bool    `json:"fallback_self_signed"`
	Storage            string   `json:"storage"`
	CAs                []ACMECA `json:"cas"`
}

// FallbackSelfSignedEnabled 报告是否允许签发失败时继续用自签证书。
func (c *AcmeConfig) FallbackSelfSignedEnabled() bool {
	if c == nil || c.FallbackSelfSigned == nil {
		return true
	}
	return *c.FallbackSelfSigned
}

// Active 报告 acme 段是否启用。缺省段（nil）视为关闭。
func (c *AcmeConfig) Active() bool {
	return c != nil && c.Enabled
}

// UsesChallenge 报告配置的挑战列表是否可能落到指定挑战。
// auto 表示由进程按可用性排序，因此对任一挑战都返回真。
func (c *AcmeConfig) UsesChallenge(name string) bool {
	if c == nil {
		return false
	}
	for _, v := range c.Challenge {
		if v == name || v == ACMEChallengeAuto {
			return true
		}
	}
	return false
}
