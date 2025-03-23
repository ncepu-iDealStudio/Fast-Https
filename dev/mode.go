package dev

import (
	"fast-https/utils/logger"
)

// Mode 表示开发模式状态
var Mode bool

// SetMode 设置开发模式
func SetMode(enabled bool) {
	// 避免重复设置
	if Mode == enabled {
		return
	}
	Mode = enabled
	if enabled {
		logger.Level(6) // 开发模式使用最详细的日志级别
		logger.Info("Running in development mode")
	}
}

// IsDevMode 返回当前是否为开发模式
func IsDevMode() bool {
	return Mode
}
