// Package logging 是进程唯一的日志出口：控制台一路、文件四路，全部输出 JSON。
//
// 本包属于基础设施层：只依赖标准库、zerolog 与 infra 内部包，
// 不允许依赖 config 或任何业务包。
package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var (
	// ConsoleLog 是进程控制台日志器，输出到标准输出。
	// 只允许在启动阶段（监听开始之前）经 SetLevel 重新赋值；
	// 服务运行期间只读。
	ConsoleLog zerolog.Logger
)

// 四路文件日志器。Open 在启动与 reload 时整体替换，
// 请求协程经 System()/Access()/Error()/Safe() 读取，
// 永远拿到完整的新旧某一个，不会读到半初始化状态。
var (
	systemLog atomic.Pointer[zerolog.Logger]
	accessLog atomic.Pointer[zerolog.Logger]
	errorLog  atomic.Pointer[zerolog.Logger]
	safeLog   atomic.Pointer[zerolog.Logger]
)

// System 返回系统日志器（记录启动、重载与运行期提示）。
func System() *zerolog.Logger { return systemLog.Load() }

// Access 返回访问日志器（逐请求，写入量最大的一路）。
func Access() *zerolog.Logger { return accessLog.Load() }

// Error 返回错误日志器。
func Error() *zerolog.Logger { return errorLog.Load() }

// Safe 返回安全日志器（限流、黑名单等安全事件）。
func Safe() *zerolog.Logger { return safeLog.Load() }

var (
	logMu    sync.Mutex
	logFiles []*os.File
)

func init() {
	zerolog.TimeFieldFormat = TimeFormat
	ConsoleLog = consoleLogger(zerolog.InfoLevel)
	// 文件日志在 Open 之前保持静默，避免零值 Logger 写入 nil writer。
	nop := zerolog.Nop()
	systemLog.Store(&nop)
	accessLog.Store(&nop)
	errorLog.Store(&nop)
	safeLog.Store(&nop)
}

// consoleLogger 构造控制台日志器。
func consoleLogger(level zerolog.Level) zerolog.Logger {
	return zerolog.New(os.Stdout).
		Level(level).
		With().
		Timestamp().
		Str(FieldKind, KindConsole).
		Logger()
}

// SetLevel 设置控制台输出级别。文件日志固定记录 debug 及以上，不受影响。
// 只允许在启动阶段调用，服务运行期间并发调用不受保护。
func SetLevel(level zerolog.Level) {
	ConsoleLog = consoleLogger(level)
}

// Open 打开四路日志文件，同时用于启动与 reload。
//
// 先打开新文件再关闭旧文件，因此失败时原有日志继续可写。
func Open(logRootPath string) error {
	files, err := openLogSet(logRootPath)
	if err != nil {
		ConsoleLog.Error().Err(err).Msg("log to file disabled")
		return err
	}
	CloseLogFiles()
	logMu.Lock()
	logFiles = files
	logMu.Unlock()
	return nil
}

// CloseLogFiles 关闭四路日志文件。
func CloseLogFiles() {
	logMu.Lock()
	defer logMu.Unlock()
	for _, file := range logFiles {
		_ = file.Close()
	}
	logFiles = nil
}

// openLogSet 打开四个文件并重建四路日志器。
func openLogSet(path string) ([]*os.File, error) {
	dir, err := mkdirLogDir(path)
	if err != nil {
		return nil, err
	}
	names := [...]string{SystemLogName, AccessLogName, ErrorLogName, SafeLogName}
	opened := make([]*os.File, 0, len(names))
	for _, name := range names {
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o666)
		if err != nil {
			for _, f := range opened {
				_ = f.Close()
			}
			return nil, err
		}
		opened = append(opened, file)
	}

	sys := fileLogger(opened[0], KindSystem)
	systemLog.Store(&sys)
	acc := fileLogger(opened[1], KindAccess)
	accessLog.Store(&acc)
	errL := fileLogger(opened[2], KindError)
	errorLog.Store(&errL)
	saf := fileLogger(opened[3], KindSafe)
	safeLog.Store(&saf)
	return opened, nil
}

// fileLogger 构造一路文件日志器，同时回显到标准输出。
func fileLogger(file *os.File, kind string) zerolog.Logger {
	return zerolog.New(&syncWriter{w: io.MultiWriter(os.Stdout, file)}).
		Level(zerolog.DebugLevel).
		With().
		Timestamp().
		Str(FieldKind, kind).
		Logger()
}

// mkdirLogDir 解析并创建日志目录。空路径、"." 与 "./" 使用 DefaultLogRoot。
func mkdirLogDir(logPath string) (string, error) {
	cleaned := strings.TrimSpace(logPath)
	if cleaned == "" || cleaned == "." || cleaned == "./" || cleaned == `.\` {
		cleaned = DefaultLogRoot
	}
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return "", err
	}
	return cleaned, nil
}

// syncWriter 串行化写入，避免多个连接协程同时写同一个文件时行交错。
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
