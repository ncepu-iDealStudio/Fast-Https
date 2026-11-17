package config

import (
	"fast-https/config/internal/acme"
	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/scan"
	"fast-https/config/internal/sitefile"
)

// 运行期配置类型。别名不引入新类型，外部包的赋值、字段访问与方法调用
// 与重构前完全一致。
type (
	Fast_Https      = model.Fast_Https
	Server          = model.Server
	Path            = model.Path
	Header          = model.Header
	Cache           = model.Cache
	PathAuth        = model.PathAuth
	PathLimit       = model.PathLimit
	ServerLimit     = model.ServerLimit
	Try             = model.Try
	Engine          = model.Engine
	WAFConfig       = model.WAFConfig
	AcmeConfig      = model.AcmeConfig
	ACMECA          = model.ACMECA
	SecurityConfig  = model.SecurityConfig
	SecurityHeaders = model.SecurityHeaders
	TLSConfig       = model.TLSConfig
	Certificate     = model.Certificate

	SiteLocation = sitefile.SiteLocation
	SiteInput    = sitefile.SiteInput
	SSLDecl      = scan.SSLDecl
)

// 编译期路径基址与派生路径。
const (
	FAST_HTTPS_ETC_DIR   = model.FAST_HTTPS_ETC_DIR
	FAST_HTTPS_DATA_DIR  = model.FAST_HTTPS_DATA_DIR
	FAST_HTTPS_LOG_DIR   = model.FAST_HTTPS_LOG_DIR
	FAST_HTTPS_RUN_DIR   = model.FAST_HTTPS_RUN_DIR
	FAST_HTTPS_STATE_DIR = model.FAST_HTTPS_STATE_DIR

	PID_FILE         = model.PID_FILE
	CONFIG_FILE_PATH = model.CONFIG_FILE_PATH
	MIME_FILE_PATH   = model.MIME_FILE_PATH

	ROOT_CRT_DIR     = model.ROOT_CRT_DIR
	CERT_DIR         = model.CERT_DIR
	ROOT_CRT_KEY_DIR = model.ROOT_CRT_KEY_DIR
	ROOT_CRT_NAME    = model.ROOT_CRT_NAME

	STATE_DIR        = model.STATE_DIR
	SYSTEM_LOG_NAME  = model.SYSTEM_LOG_NAME
	ACCESS_LOG_NAME  = model.ACCESS_LOG_NAME
	ERROR_LOG_NAME   = model.ERROR_LOG_NAME
	SAFE_LOG_NAME    = model.SAFE_LOG_NAME
	DEFAULT_LOG_ROOT = model.DEFAULT_LOG_ROOT
)

// 其它运行期常量。
const (
	CURRENT_VERSION = model.CURRENT_VERSION

	DEFAULT_PORT            = model.DEFAULT_PORT
	DEFAULT_MAX_HEADER_SIZE = model.DEFAULT_MAX_HEADER_SIZE
	DEFAULT_MAX_BODY_SIZE   = model.DEFAULT_MAX_BODY_SIZE

	HTTP_DEFAULT_CONTENT_TYPE = model.HTTP_DEFAULT_CONTENT_TYPE
)

// location type / 压缩编码 / 请求头来源。
const (
	ZIP_NONE    = model.ZIP_NONE
	ZIP_GZIP    = model.ZIP_GZIP
	ZIP_BR      = model.ZIP_BR
	ZIP_GZIP_BR = model.ZIP_GZIP_BR

	Host        = model.Host
	XRealIp     = model.XRealIp
	XForwardFor = model.XForwardFor

	LOCAL       = model.LOCAL
	PROXY_HTTP  = model.PROXY_HTTP
	PROXY_HTTPS = model.PROXY_HTTPS
	PROXY_TCP   = model.PROXY_TCP
	REWRITE     = model.REWRITE
	DEVMOD      = model.DEVMOD
)

// acme 段常量。
const (
	ACMEChallengeAuto      = model.ACMEChallengeAuto
	ACMEChallengeHTTP01    = model.ACMEChallengeHTTP01
	ACMEChallengeTLSALPN01 = model.ACMEChallengeTLSALPN01

	ACMELetsEncryptDirectory        = model.ACMELetsEncryptDirectory
	ACMELetsEncryptStagingDirectory = model.ACMELetsEncryptStagingDirectory
	ACMEZeroSSLDirectory            = model.ACMEZeroSSLDirectory

	DefaultACMERenewBeforeDays = model.DefaultACMERenewBeforeDays
	ACMECertStorageDir         = model.ACMECertStorageDir
)

// security 段与符号链接策略常量。
const (
	HostCheckOff     = model.HostCheckOff
	HostCheckWarn    = model.HostCheckWarn
	HostCheckEnforce = model.HostCheckEnforce

	SYMLINK_ALLOW         = model.SYMLINK_ALLOW
	SYMLINK_REJECT        = model.SYMLINK_REJECT
	SYMLINK_CHECK_AT_LOAD = model.SYMLINK_CHECK_AT_LOAD
)

// 候选树与站点快照的哨兵错误。
var (
	ErrCandidatePath = paths.ErrCandidatePath
	ErrNoServerBlock = sitefile.ErrNoServerBlock
)

// DefaultACMECAs 返回默认 CA 降级链。
func DefaultACMECAs() []ACMECA { return acme.DefaultACMECAs() }
