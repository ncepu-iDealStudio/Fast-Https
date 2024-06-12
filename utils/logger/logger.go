package logger

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type LogLevel int

var loglevel LogLevel

func Level(level int) {
	loglevel = LogLevel(level)
}

const (
	FATAL LogLevel = iota
	ERROR
	WARN
	NOTICE
	INFO
	DEBUG
	TRACE
)

func Help() string {
	return "log level	(0 FATAL, 1 ERROR, 2 WARN, 3 NOTICE, 4 INFO, 5 DEBUG, 6 TRACE)"
}

func (level LogLevel) String() string {
	names := []string{
		"FATAL",
		"ERROR",
		"WARN",
		"NOTICE",
		"INFO",
		"DEBUG",
		"TRACE",
	}
	return names[int(level)]
}

func getPath() (dir, path string, line int) {
	_, fullpath, line, _ := runtime.Caller(3)
	f := strings.Split(fullpath, "/")
	dir = f[len(f)-2]
	file := f[len(f)-1]
	path = dir + "/" + file
	return
}

func out(level LogLevel, format string, v ...interface{}) {
	_, path, line := getPath()

	// TODO: always stderr in debug
	dest := os.Stdout
	if level >= 4 {
		dest = os.Stderr
	}
	//dest := os.Stderr

	if level > loglevel {
		return
	}

	fmt.Fprintf(dest,
		"%s [%v:%v] %v\n",
		level,
		path,
		line,
		fmt.Sprintf(format, v...))

	// in Logger.Fatal() case
	if level == FATAL {
		fmt.Println("exit")
		os.Exit(1)
	}
}

func Fatal(format string, v ...interface{}) {
	out(FATAL, format, v...)
}

func Error(formt string, v ...interface{}) {
	out(ERROR, formt, v...)
}

func Warn(format string, v ...interface{}) {
	out(WARN, format, v...)
}

func Notice(formt string, v ...interface{}) {
	out(NOTICE, formt, v...)
}

func Info(formt string, v ...interface{}) {
	out(INFO, formt, v...)
}

func Debug(formt string, v ...interface{}) {
	out(DEBUG, formt, v...)
}

func Trace(format string, v ...interface{}) {
	out(TRACE, format, v...)
}
