package config

const (
	/* version */
	CURRENT_VERSION string = "V1.2.1"

	/*
		// use "config_dev/fast-https.json" when dev
		// use "config/fast-https.json" when release
	*/
	PID_FILE               string = "./fast-https.pid"
	CONFIG_FILE_PATH       string = "./config/fast-https.json"
	MIME_FILE_PATH         string = "./config/mime.types"
	MONIITOR_LOG_FILE_PATH string = "./logs/monitor.log"

	/* events */
	DEFAULT_PORT string = ":8080"

	/* log message*/
	SERVER_TIME_FORMAT string = "2006-01-02 15:04:05"
	SYSTEM_LOG_NAME    string = "system.log"
	ACCESS_LOG_NAME    string = "access.log"
	ERROR_LOG_NAME     string = "error.log"
	SAFE_LOG_NAME      string = "safe.log"
)

/*
cd monitor &&
go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go &&
echo "monitor compiler successed" &&
cd .. &&
goreleaser release -f .goreleaser.windows.yaml --snapshot --clean &&
goreleaser release -f .goreleaser.yaml --snapshot --clean
*/

/*
http://127.0.0.1:10000/debug/pprof/
go tool pprof main http://localhost:10000/debug/pprof/heap   web
*/