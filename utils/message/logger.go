package message

import (
	"fast-https/config"
	"fast-https/utils/logger"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	Glog = &Logs{}
	// TODO: server reload
	logOnce  sync.Once
	logFiles []*os.File
)

type Logs struct {
	SystemLog *logrus.Logger
	AccessLog *logrus.Logger
	ErrorLog  *logrus.Logger
	SafeLog   *logrus.Logger
}

func MessageFormat(path string) {
	logOnce.Do(func() {
		Glog.SystemLog = loggerToFileAndCmd(path, config.SYSTEM_LOG_NAME)
		Glog.SystemLog.SetFormatter(&SystemLogFormatter{})

		Glog.AccessLog = loggerToFileAndCmd(path, config.ACCESS_LOG_NAME)
		Glog.AccessLog.SetFormatter(&AccessLogFormatter{})

		Glog.ErrorLog = loggerToFileAndCmd(path, config.ERROR_LOG_NAME)
		Glog.ErrorLog.SetFormatter(&ErrorLogFormatter{})

		Glog.SafeLog = loggerToFileAndCmd(path, config.SAFE_LOG_NAME)
		Glog.SafeLog.SetFormatter(&SafeLogFormatter{})
	})
}

// ResolveLogDir picks the directory for access.log, error.log, safe.log and system.log.
// An empty path, "." or "./" uses config.DEFAULT_LOG_ROOT (./logs). The directory is created when missing.
//
// Console logs from utils/logger use a different rule: DEBUG and TRACE go to stderr, and lower levels go to stdout.
// Those lines are not written into the four files above.
func ResolveLogDir(logPath string) string {
	cleaned := strings.TrimSpace(logPath)
	if cleaned == "" || cleaned == "." || cleaned == "./" || cleaned == `.\` {
		cleaned = config.DEFAULT_LOG_ROOT
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		logger.Warn("create log dir %s: %v", cleaned, err)
	}
	return cleaned
}

func loggerToFileAndCmd(logPath string, logName string) *logrus.Logger {
	fileName := filepath.Join(ResolveLogDir(logPath), logName)
	// 写入文件
	src, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		logger.Warn("log to file err: %v", err)
	} else {
		logFiles = append(logFiles, src)
	}
	// 实例化
	logger := logrus.New()
	// 设置输出
	// TODO:
	fileAndStdoutWriter := io.MultiWriter(os.Stdout, src)
	logger.SetOutput(fileAndStdoutWriter)
	// 设置日志级别
	logger.SetLevel(logrus.DebugLevel)
	//// 设置 rotatelogs
	//logWriter, err := rotatelogs.New(
	//	// 分割后的文件名称
	//	fileName+".%Y%m%d.log",
	//
	//	// 生成软链，指向最新日志文件
	//	rotatelogs.WithLinkName(fileName),
	//
	//	// 设置最大保存时间(7天)
	//	rotatelogs.WithMaxAge(7*24*time.Hour),
	//
	//	// 设置日志切割时间间隔(1天)
	//	rotatelogs.WithRotationTime(24*time.Hour),
	//)
	//
	//writeMap := lfshook.WriterMap{
	//	logrus.InfoLevel:  logWriter,
	//	logrus.FatalLevel: logWriter,
	//	logrus.DebugLevel: logWriter,
	//	logrus.WarnLevel:  logWriter,
	//	logrus.ErrorLevel: logWriter,
	//	logrus.PanicLevel: logWriter,
	//}
	//
	//lfHook := lfshook.NewHook(writeMap, &logrus.JSONFormatter{
	//	TimestampFormat: "2006-01-02 15:04:05",
	//})
	//
	//// 新增 Hook
	//logger.AddHook(lfHook)

	return logger
}

// CloseLogFiles closes files opened for the four server logs.
func CloseLogFiles() {
	for _, file := range logFiles {
		_ = file.Close()
	}
	logFiles = nil
}
