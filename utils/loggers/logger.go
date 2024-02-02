package loggers

import (
	"fast-https/config"
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	log     = &Logs{}
	logOnce sync.Once
)

type Logs struct {
	systemLog *logrus.Logger
	accessLog *logrus.Logger
	errorLog  *logrus.Logger
	safeLog   *logrus.Logger
}

func (l *Logs) SystemLog() *logrus.Logger {
	return l.systemLog
}

func (l *Logs) AccessLog() *logrus.Logger {
	return l.accessLog
}

func (l *Logs) ErrorLog() *logrus.Logger {
	return l.errorLog
}

func (l *Logs) SafeLog() *logrus.Logger {
	return l.safeLog
}

func GetLogger() *Logs {
	return log
}

func InitLogger(path string) {
	logOnce.Do(func() {
		log.systemLog = loggerToFileAndCmd(path, config.SYSTEM_LOG_NAME)
		log.systemLog.SetFormatter(&SystemLogFormatter{})
		log.accessLog = loggerToFileAndCmd(path, config.ACCESS_LOG_NAME)
		log.accessLog.SetFormatter(&AccessLogFormatter{})
		log.errorLog = loggerToFileAndCmd(path, config.ERROR_LOG_NAME)
		log.errorLog.SetFormatter(&ErrorLogFormatter{})
		log.safeLog = loggerToFileAndCmd(path, config.SAFE_LOG_NAME)
		log.safeLog.SetFormatter(&SafeLogFormatter{})
	})
}

func loggerToFileAndCmd(logPath string, logName string) *logrus.Logger {
	// 日志文件
	fileName := path.Join(logPath, logName)

	// 写入文件
	src, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Println("log to file err:", err)
	}

	// 实例化
	logger := logrus.New()

	// 设置输出
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

func loggerToCmd() *logrus.Logger {
	logger := logrus.New()
	logger.Out = os.Stdout
	// 设置日志级别
	logger.SetLevel(logrus.DebugLevel)
	return logger
}
