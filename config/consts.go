package config

import "fast-https/infra/logging"

const (
	/* version */
	CURRENT_VERSION string = "V1.4.0"

	/*
		// use "config_dev/fast-https.json" when dev
		// use "config/fast-https.json" when release
	*/
	PID_FILE         string = FAST_HTTPS_BASE_DIR + "fast-https.pid"
	CONFIG_FILE_PATH string = FAST_HTTPS_BASE_DIR + "config/fast-https.json"
	MIME_FILE_PATH   string = FAST_HTTPS_BASE_DIR + "config/mime.json"

	/* events */
	DEFAULT_PORT            string = ":8080"
	DEFAULT_MAX_HEADER_SIZE        = 4096
	DEFAULT_MAX_BODY_SIZE          = 512 * 1024 // 32K

	DEFAULT_LOG_ROOT       string = FAST_HTTPS_BASE_DIR + logging.DefaultLogRoot
	MONIITOR_LOG_FILE_PATH string = "monitor.log"

	/* log message*/
	SYSTEM_LOG_NAME string = logging.SystemLogName
	ACCESS_LOG_NAME string = logging.AccessLogName
	ERROR_LOG_NAME  string = logging.ErrorLogName
	SAFE_LOG_NAME   string = logging.SafeLogName
)

const (
	HTTP_DEFAULT_CONTENT_TYPE = "application/octet-stream"
)

const (
	// 站点根。httpdoc/root/index.html 用 <a href="root.crt"> 提供自签 CA 下载，
	// 所以 CA 证书本身留在这里，对外可见。
	ROOT_CRT_DIR = FAST_HTTPS_BASE_DIR + "httpdoc/root"
	// 服务器证书目录，CA 私钥也放这里。私钥不能落在站点根下：
	// 静态模块只拦 ../ 逃逸，httpdoc/root/root.key 能被直接下载。
	CERT_DIR         = FAST_HTTPS_BASE_DIR + "config/cert"
	ROOT_CRT_KEY_DIR = CERT_DIR
	ROOT_CRT_NAME    = "root"
)

/*
goreleaser release -f build/goreleaser.yaml --snapshot --clean
*/

/*
http://127.0.0.1:10000/debug/pprof/
go tool pprof main http://localhost:10000/debug/pprof/heap   web
*/
