package loggers

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
)

type SystemLogFormatter struct {
}

type AccessLogFormatter struct {
}

type ErrorLogFormatter struct {
}

type SafeLogFormatter struct {
}

func (s *SystemLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string
	newLog = fmt.Sprintf("%s %s\n", timestamp, entry.Message)

	b.WriteString(newLog)
	return b.Bytes(), nil
}

func (a *AccessLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string
	newLog = fmt.Sprintf("%s -- [%s] %v\n", entry.Data["host"], timestamp, entry.Message)

	b.WriteString(newLog)
	return b.Bytes(), nil
}

func (e *ErrorLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string
	newLog = fmt.Sprintf("%s [%s] %v %s\n", timestamp, entry.Level, entry.Caller, entry.Message)

	b.WriteString(newLog)
	return b.Bytes(), nil
}

func (e *SafeLogFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string
	newLog = fmt.Sprintf("%s %s\n", timestamp, entry.Message)

	b.WriteString(newLog)
	return b.Bytes(), nil
}
