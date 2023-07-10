package loggers

import (
	"fmt"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/olivere/elastic/v7"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/sohlich/elogrus.v7"
	"os"
	"path"
	"sync"
	"time"
)

var (
	log     *logrus.Logger
	logOnce sync.Once
)

func GetLogger() *logrus.Logger {
	logOnce.Do(func() {
		log = loggerToCmd()
		log.Infoln("日志初始化服务完成!")
	})
	return log
}
func InitLogger(logType string) {
	logOnce.Do(func() {
		log = loggerToCmd()
	})
	if logType == "File" {
		log = loggerToFile()
	} else if logType == "ES" {
		log = loggerToES()
	} else {
		log = loggerToCmd()
	}
	log.Infoln("日志初始化服务完成!")
}

// 日志记录到文件
func loggerToFile() *logrus.Logger {
	basePath, _ := os.Getwd()
	logFilePath := path.Join(basePath, viper.GetString("log.filepath"))
	logFileName := viper.GetString("log.filename")
	//logFilePath := path.Join(basePath, "logs")
	//logFileName := "system.log"

	// 日志文件
	fileName := path.Join(logFilePath, logFileName)

	// 写入文件
	src, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		fmt.Println("err", err)
	}

	// 实例化
	logger := logrus.New()

	// 设置输出
	logger.Out = src

	// 设置日志级别
	logger.SetLevel(logrus.DebugLevel)

	// 设置 rotatelogs
	logWriter, err := rotatelogs.New(
		// 分割后的文件名称
		fileName+".%Y%m%d.log",

		// 生成软链，指向最新日志文件
		rotatelogs.WithLinkName(fileName),

		// 设置最大保存时间(7天)
		rotatelogs.WithMaxAge(7*24*time.Hour),

		// 设置日志切割时间间隔(1天)
		rotatelogs.WithRotationTime(24*time.Hour),
	)

	writeMap := lfshook.WriterMap{
		logrus.InfoLevel:  logWriter,
		logrus.FatalLevel: logWriter,
		logrus.DebugLevel: logWriter,
		logrus.WarnLevel:  logWriter,
		logrus.ErrorLevel: logWriter,
		logrus.PanicLevel: logWriter,
	}

	lfHook := lfshook.NewHook(writeMap, &logrus.JSONFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
	})

	// 新增 Hook
	logger.AddHook(lfHook)

	return logger
}

// todo 日志记录到 ES
func loggerToES() *logrus.Logger {
	// 实例化
	logger := logrus.New()
	logger.Formatter = &logrus.JSONFormatter{}

	// 设置输出
	logger.Out = os.Stdout

	// 设置日志级别
	logger.SetLevel(logrus.DebugLevel)

	// 创建elasticsearch客户端
	client, err := elastic.NewClient(elastic.SetSniff(false), elastic.SetURL(viper.GetString("remote.ESUrl")))
	if err != nil {
		logger.Panic(err)
	}
	// 将logrus和elastic绑定，host 是指定该程序执行时的ip
	hook, err := elogrus.NewElasticHook(client,
		viper.GetString("system.SysIP")+":"+viper.GetString("system.SysPort"),
		logger.Level,
		"log_"+viper.GetString("system.Name"),
	)
	if err != nil {
		logger.Panic(err)
	}
	logger.AddHook(hook)

	return logger
}

// todo 日志记录到 MQ
func loggerToMQ() *logrus.Logger {
	return nil
}

// 记录日志到控制台
func loggerToCmd() *logrus.Logger {
	logger := logrus.New()
	logger.Out = os.Stdout
	// 设置日志级别
	logger.SetLevel(logrus.DebugLevel)
	return logger
}
