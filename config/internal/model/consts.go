package model

import "fast-https/infra/logging"

const (
	/* version */
	CURRENT_VERSION string = "V1.7.1"

	/*
		// use "config_dev/fast-https.json" when dev
		// use "config/fast-https.json" when release
	*/
	// 路径按内容性质分到四个基址（见 base_dir.go / base_dir_rpm.go）：
	// pid 属运行期临时文件，配置与 MIME 表属运维文件，站点属只读数据，日志属可变状态。
	// 归档布局下四个基址仍映射到 旧版 的相对路径；包布局分属 /etc、/usr/share、
	// /var/log、/run。
	PID_FILE         string = FAST_HTTPS_RUN_DIR + "/fast-https.pid"
	CONFIG_FILE_PATH string = FAST_HTTPS_ETC_DIR + "/fast-https.json"
	MIME_FILE_PATH   string = FAST_HTTPS_ETC_DIR + "/mime.json"

	/* events */
	DEFAULT_PORT            string = ":8080"
	DEFAULT_MAX_HEADER_SIZE        = 4096
	DEFAULT_MAX_BODY_SIZE          = 512 * 1024 // 32K
	// 超时单位均为秒；0 表示用这里的默认值。
	DEFAULT_READ_TIMEOUT  = 30
	DEFAULT_WRITE_TIMEOUT = 30
	DEFAULT_BODY_TIMEOUT  = 1
	// DEFAULT_UPSTREAM_TIMEOUT 是代理到上游的整体超时（秒）。
	DEFAULT_UPSTREAM_TIMEOUT = 20

	DEFAULT_LOG_ROOT string = FAST_HTTPS_LOG_DIR
	STATE_DIR        string = FAST_HTTPS_STATE_DIR

	/* log message*/
	SYSTEM_LOG_NAME string = logging.SystemLogName
	ACCESS_LOG_NAME string = logging.AccessLogName
	ERROR_LOG_NAME  string = logging.ErrorLogName
	SAFE_LOG_NAME   string = logging.SafeLogName
)

const (
	HTTP_DEFAULT_CONTENT_TYPE = "application/octet-stream"
)

// ACME 挑战名、默认 CA 目录与默认值。
const (
	// ACMEChallengeAuto 表示由进程按可用性自动排序挑战，默认值。
	ACMEChallengeAuto      = "auto"
	ACMEChallengeHTTP01    = "http-01"
	ACMEChallengeTLSALPN01 = "tls-alpn-01"

	// 默认降级链：先 Let's Encrypt，失败再 ZeroSSL，全部失败回落本地自签。
	ACMELetsEncryptDirectory        = "https://acme-v02.api.letsencrypt.org/directory"
	ACMELetsEncryptStagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ACMEZeroSSLDirectory            = "https://acme.zerossl.com/v2/DV90"

	DefaultACMERenewBeforeDays = 30

	// ACMECertStorageDir 相对 ETC 基址，落在证书目录下的 acme 子目录：
	// 归档是 ./config/cert/acme，包是 /etc/fast-https/cert/acme。账户私钥与
	// 证书缓存同处一棵树，整目录拷走就能在另一台机器继续签发与续期。
	ACMECertStorageDir = "./cert/acme"
)

const (
	// 站点根。httpdoc/root/index.html 用 <a href="root.crt"> 提供自签 CA 下载，
	// 所以 CA 证书本身留在这里，对外可见。
	ROOT_CRT_DIR = FAST_HTTPS_DATA_DIR + "/httpdoc/root"
	// 服务器证书目录，CA 私钥也放这里。私钥不能落在站点根下：
	// 静态模块只拦 ../ 逃逸，httpdoc/root/root.key 能被直接下载。
	CERT_DIR         = FAST_HTTPS_ETC_DIR + "/cert"
	ROOT_CRT_KEY_DIR = CERT_DIR
	ROOT_CRT_NAME    = "root"
)

/*
goreleaser release -f build/goreleaser.yaml --snapshot --clean
*/
