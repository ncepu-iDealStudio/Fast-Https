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
		_ = openLogs(path)
	})
}

// Reopen closes the current log files and opens them again under path.
// A failure leaves the previous files in place.
func Reopen(path string) error {
	logs, files, err := openLogSet(path)
	if err != nil {
		logger.Error("reload kept previous log files: %v", err)
		return err
	}
	CloseLogFiles()
	logFiles = files
	Glog.SystemLog = logs.SystemLog
	Glog.AccessLog = logs.AccessLog
	Glog.ErrorLog = logs.ErrorLog
	Glog.SafeLog = logs.SafeLog
	return nil
}

func openLogs(path string) error {
	logs, files, err := openLogSet(path)
	if err != nil {
		logger.Warn("log to file err: %v", err)
		return err
	}
	logFiles = append(logFiles, files...)
	Glog.SystemLog = logs.SystemLog
	Glog.AccessLog = logs.AccessLog
	Glog.ErrorLog = logs.ErrorLog
	Glog.SafeLog = logs.SafeLog
	return nil
}

func openLogSet(path string) (Logs, []*os.File, error) {
	dir, err := mkdirLogDir(path)
	if err != nil {
		return Logs{}, nil, err
	}
	names := []string{
		config.SYSTEM_LOG_NAME,
		config.ACCESS_LOG_NAME,
		config.ERROR_LOG_NAME,
		config.SAFE_LOG_NAME,
	}
	opened := make([]*os.File, 0, len(names))
	loggers := make([]*logrus.Logger, 0, len(names))
	for _, name := range names {
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			for _, openedFile := range opened {
				_ = openedFile.Close()
			}
			return Logs{}, nil, err
		}
		opened = append(opened, file)
		lg := logrus.New()
		lg.SetOutput(io.MultiWriter(os.Stdout, file))
		lg.SetLevel(logrus.DebugLevel)
		loggers = append(loggers, lg)
	}
	loggers[0].SetFormatter(&SystemLogFormatter{})
	loggers[1].SetFormatter(&AccessLogFormatter{})
	loggers[2].SetFormatter(&ErrorLogFormatter{})
	loggers[3].SetFormatter(&SafeLogFormatter{})
	return Logs{
		SystemLog: loggers[0],
		AccessLog: loggers[1],
		ErrorLog:  loggers[2],
		SafeLog:   loggers[3],
	}, opened, nil
}

func mkdirLogDir(logPath string) (string, error) {
	cleaned := strings.TrimSpace(logPath)
	if cleaned == "" || cleaned == "." || cleaned == "./" || cleaned == `.\` {
		cleaned = config.DEFAULT_LOG_ROOT
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return "", err
	}
	return cleaned, nil
}

// ResolveLogDir picks the directory for access.log, error.log, safe.log and system.log.
// An empty path, "." or "./" uses config.DEFAULT_LOG_ROOT (./logs). The directory is created when missing.
//
// Console logs from utils/logger use a different rule: DEBUG and TRACE go to stderr, and lower levels go to stdout.
// Those lines are not written into the four files above.
func ResolveLogDir(logPath string) string {
	dir, err := mkdirLogDir(logPath)
	if err != nil {
		logger.Warn("create log dir %s: %v", logPath, err)
		if strings.TrimSpace(logPath) == "" {
			return config.DEFAULT_LOG_ROOT
		}
		return logPath
	}
	return dir
}

// CloseLogFiles closes files opened for the four server logs.
func CloseLogFiles() {
	for _, file := range logFiles {
		_ = file.Close()
	}
	logFiles = nil
}
