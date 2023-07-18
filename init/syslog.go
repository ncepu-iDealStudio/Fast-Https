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

func monitorLog(logChan chan string) {
	var logData []string
	var lastWrite time.Time

	for {
		select {
		case log := <-logChan:
			logData = append(logData, log)
			if len(logData) >= 1 || time.Since(lastWrite) >= time.Duration(maxWait)*time.Millisecond {
				writeToFile(logData)
				lastWrite = time.Now()
				logData = nil
			}
		case <-time.Tick(time.Duration(maxWait) * time.Millisecond):
			if len(logData) > 0 {
				writeToFile(logData)
				lastWrite = time.Now()
				logData = nil
			}
		}
	}
}

func writeLog(logChan chan string) {
	for i := 0; i < 100000; i++ {
		logChan <- fmt.Sprintf("%v %v\n", time.Now().Format("2006-01-02 15:04:05"), "log message")
		time.Sleep(10 * time.Millisecond)
	}
}

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

func SysLogInit() {
	logChan := make(chan string, 1000)
	go monitorLog(logChan)
	go writeLog(logChan)
	select {}
}
