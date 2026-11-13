// Package config 负责加载、校验并热重载 JSON 配置（含 conf.d 包含目录）。
//
// 文件分工：
//   - config.go     运行期类型、配置快照与加载入口（本文件）
//   - schema.go     配置文件的 JSON schema 与数字/字符串/数组混写兼容
//   - convert.go    schema → 运行期结构的转换与默认值
//   - validate.go   check 与 reload 路径的完整校验
//   - paths.go      相对路径解析与 MIME 表
//   - site.go       站点文件（conf.d/<name>.json）的读写支撑
//   - acme.go       acme 段配置与默认值；acme_write.go 负责写回 acme 段
//   - security.go   security 段（Host 校验与安全响应头）
package config

import (
	"strings"
	"sync/atomic"
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
	PathName       string
	PathType       uint16
	Zip            uint16
	Root           string
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
}

// 运行时配置快照。reload 校验通过后整体替换快照，读取端要么看到完整的
// 旧配置、要么看到完整的新配置，失败时旧快照保持不动。
// （架构收敛计划阶段 4b：消除对进程级全局配置的无同步读写。）
var (
	currentConfig atomic.Pointer[Fast_Https]
	currentMIME   atomic.Pointer[map[string]string]
)

func init() {
	currentConfig.Store(&Fast_Https{})
	emptyMIME := map[string]string{}
	currentMIME.Store(&emptyMIME)
}

// Current 返回当前生效的配置快照。尚未加载配置时是零值快照。
func Current() *Fast_Https {
	return currentConfig.Load()
}

// MIME 返回当前生效的 MIME 表。表在发布后不再修改，读取方无需加锁。
func MIME() map[string]string {
	return *currentMIME.Load()
}

// Store 替换配置快照。LoadFile/ReloadFile 在校验全部通过后调用；
// 导出是为了测试能够保存与恢复快照。
func Store(cfg *Fast_Https, mime map[string]string) {
	if cfg != nil {
		currentConfig.Store(cfg)
	}
	if mime != nil {
		currentMIME.Store(&mime)
	}
}

// Init the whole config module
func Init() error {
	return LoadFile(CONFIG_FILE_PATH)
}

// LoadFile reads one config file and the MIME table, then replaces the
// config snapshot with both. Neither changes on error, and readers never
// observe a cleared or half-filled value.
func LoadFile(path string) error {
	cfg, err := processRoot(path)
	if err != nil {
		return err
	}
	mime, err := loadContentTypes(path)
	if err != nil {
		return err
	}
	Store(&cfg, mime)
	return nil
}

// Reload reloads the runtime config file.
// An invalid file leaves the current snapshot unchanged.
func Reload() error {
	return ReloadFile(CONFIG_FILE_PATH)
}

// ReloadFile validates path before replacing the config snapshot.
// Validation or load errors leave the previous config in place.
func ReloadFile(path string) error {
	if err := ValidateConfigFile(path); err != nil {
		return err
	}
	return LoadFile(path)
}
