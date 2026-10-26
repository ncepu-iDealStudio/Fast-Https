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
	ROOT_CRT_DIR  = FAST_HTTPS_BASE_DIR + "httpdoc/root"
	CERT_DIR      = FAST_HTTPS_BASE_DIR + "config/cert"
	ROOT_CRT_NAME = "root"
)

/*
goreleaser release -f .goreleaser.yaml --snapshot --clean
*/

/*
http://127.0.0.1:10000/debug/pprof/
go tool pprof main http://localhost:10000/debug/pprof/heap   web
*/
