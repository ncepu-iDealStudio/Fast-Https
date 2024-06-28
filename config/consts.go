package config

const (
	/* version */
	CURRENT_VERSION string = "V1.3.2"

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

	DEFAULT_LOG_ROOT       string = FAST_HTTPS_BASE_DIR + "logs"
	MONIITOR_LOG_FILE_PATH string = "monitor.log"

	/* log message*/
	SERVER_TIME_FORMAT string = "2006-01-02 15:04:05"
	SYSTEM_LOG_NAME    string = "system.log"
	ACCESS_LOG_NAME    string = "access.log"
	ERROR_LOG_NAME     string = "error.log"
	SAFE_LOG_NAME      string = "safe.log"
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
cd monitor &&
go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go windows.go &&
echo "monitor compiler successed" &&
cd .. &&
goreleaser release -f .goreleaser.windows.yaml --snapshot --clean &&
goreleaser release -f .goreleaser.yaml --snapshot --clean
*/

/*
http://127.0.0.1:10000/debug/pprof/
go tool pprof main http://localhost:10000/debug/pprof/heap   web
*/
