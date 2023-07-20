package init

import (
	"fmt"
	"os"
	"time"
)

// 以下需转移到配置文件中
var (
	logFile = "log.txt"
	maxSize = 1.024 * 1.024
	maxWait = 5000 // 5s
)

// monitorLog monitor log data in logChan
func monitorLog(logChan chan string) {
	var logData []string
	var lastWrite time.Time

	timer := time.NewTicker(time.Duration(maxWait) * time.Millisecond)
	defer timer.Stop()
	defer close(logChan)

	for {
		select {
		case log := <-logChan:
			logData = append(logData, log)
			if len(logData) >= 1 || time.Since(lastWrite) >= time.Duration(maxWait)*time.Millisecond {
				writeToFile(logData)
				lastWrite = time.Now()
				logData = nil
			}
		case <-timer.C:
			if len(logData) > 0 {
				writeToFile(logData)
				lastWrite = time.Now()
				logData = nil
			}
		}
	}
}

// WriteLog writes the log to logChan
func writeLog(logChan chan string, input string) {
	logChan <- fmt.Sprintln(input)
}

// writeToFile  write  log to file
func writeToFile(logData []string) {
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()
	for _, log := range logData {
		_, err := file.WriteString(log)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return
		}
	}
}

// SysLogInit SysLog handle initial
func SysLogInit() {
	logChan := make(chan string, 1000)
	go monitorLog(logChan)
}
