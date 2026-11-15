// Package logging 是进程唯一的日志出口：控制台一路、文件四路，全部输出 JSON。
//
// 本包属于基础设施层：只依赖标准库、zerolog 与 infra 内部包，
// 不允许依赖 config 或任何业务包。
package logging

import (
	"bytes"
	"encoding/json"
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
	// errorFile 是 error.log 的文件写入器（不含 stdout 回显）。
	// ConsoleLog 的 Warn 及以上往这里镜像；Open 之前为 nil，不缓冲。
	errorFile atomic.Pointer[syncWriter]
)

// consoleEcho 控制四路文件日志是否同时回显到标准输出。
// 后台子进程把标准输出指向 logs/daemon.log 时必须关掉：否则同一行写两遍，
// 访问日志还会把 daemon.log 一路撑大，等于多一份没有轮转的副本。
var consoleEcho atomic.Bool

// SetConsoleEcho 开关文件日志的标准输出回显。只允许在启动阶段、
// logging.Open 之前调用（后台子进程在 exec 后立刻设置）。
func SetConsoleEcho(on bool) { consoleEcho.Store(on) }

func init() {
	zerolog.TimeFieldFormat = TimeFormat
	ConsoleLog = consoleLogger(zerolog.InfoLevel)
	consoleEcho.Store(true)
	// 文件日志在 Open 之前保持静默，避免零值 Logger 写入 nil writer。
	nop := zerolog.Nop()
	systemLog.Store(&nop)
	accessLog.Store(&nop)
	errorLog.Store(&nop)
	safeLog.Store(&nop)
}

// consoleOut 与 consoleLevel 记住控制台日志器当前的目标和级别，
// 这样 SetLevel 与 SetConsoleOutput 各改各的，互不覆盖。
var (
	consoleOut   io.Writer = os.Stdout
	consoleLevel           = zerolog.InfoLevel
)

// consoleLogger 构造控制台日志器。写出时经 consoleWriter：原样到控制台，
// Warn 及以上再镜像一份到 error.log（kind=error, source=console）。
func consoleLogger(level zerolog.Level) zerolog.Logger {
	return zerolog.New(consoleWriter{}).
		Level(level).
		With().
		Timestamp().
		Str(FieldKind, KindConsole).
		Logger()
}

// consoleWriter 把控制台日志写到当前 consoleOut，并按级别镜像到 error.log。
type consoleWriter struct{}

func (consoleWriter) Write(p []byte) (int, error) {
	n, err := consoleOut.Write(p)
	mirrorConsoleToError(p)
	return n, err
}

func mirrorConsoleToError(p []byte) {
	if !consoleLevelMirrored(p) {
		return
	}
	w := errorFile.Load()
	if w == nil {
		return
	}
	line := rewriteErrorMirror(p)
	if len(line) == 0 {
		return
	}
	_, _ = w.Write(line)
}

func consoleLevelMirrored(p []byte) bool {
	i := bytes.Index(p, []byte(`"level":"`))
	if i < 0 {
		return false
	}
	rest := p[i+len(`"level":"`):]
	return bytes.HasPrefix(rest, []byte(`warn"`)) ||
		bytes.HasPrefix(rest, []byte(`error"`)) ||
		bytes.HasPrefix(rest, []byte(`fatal"`)) ||
		bytes.HasPrefix(rest, []byte(`panic"`))
}

func rewriteErrorMirror(p []byte) []byte {
	var rec map[string]any
	if err := json.Unmarshal(p, &rec); err != nil {
		return nil
	}
	rec[FieldKind] = KindError
	rec[FieldSource] = SourceConsole
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return nil
	}
	return buf.Bytes()
}

// SetLevel 设置控制台输出级别。文件日志固定记录 debug 及以上，不受影响。
// 只允许在启动阶段调用，服务运行期间并发调用不受保护。
func SetLevel(level zerolog.Level) {
	consoleLevel = level
	ConsoleLog = consoleLogger(level)
}

// SetConsoleOutput 改变控制台日志的输出目标。CLI 的 --json 模式用它把日志
// 挪到标准错误，保证标准输出只有一个 JSON 文档。
// 只允许在启动阶段、logging.Open 之前调用，约束与 SetLevel 相同。
func SetConsoleOutput(w io.Writer) {
	if w == nil {
		w = os.Stdout
	}
	consoleOut = w
	ConsoleLog = consoleLogger(consoleLevel)
}

// Open 打开四路日志文件，同时用于启动与 reload。
//
// 先打开新文件再关闭旧文件，因此失败时原有日志继续可写。
func Open(logRootPath string) error {
	files, errW, err := openLogSet(logRootPath)
	if err != nil {
		ConsoleLog.Error().Err(err).Msg("log to file disabled")
		return err
	}
	CloseLogFiles()
	if errW != nil {
		errorFile.Store(errW)
	}
	logMu.Lock()
	logFiles = files
	logMu.Unlock()
	return nil
}

// CloseLogFiles 关闭四路日志文件。
func CloseLogFiles() {
	errorFile.Store(nil)
	logMu.Lock()
	defer logMu.Unlock()
	for _, file := range logFiles {
		_ = file.Close()
	}
	logFiles = nil
}

// openLogSet 打开四个文件并重建四路日志器。
func openLogSet(path string) ([]*os.File, *syncWriter, error) {
	dir, err := mkdirLogDir(path)
	if err != nil {
		return nil, nil, err
	}
	names := [...]string{SystemLogName, AccessLogName, ErrorLogName, SafeLogName}
	opened := make([]*os.File, 0, len(names))
	for _, name := range names {
		file, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o666)
		if err != nil {
			for _, f := range opened {
				_ = f.Close()
			}
			return nil, nil, err
		}
		opened = append(opened, file)
	}

	sys := fileLogger(opened[0], KindSystem)
	systemLog.Store(&sys)
	acc := fileLogger(opened[1], KindAccess)
	accessLog.Store(&acc)
	errFile := &syncWriter{w: opened[2]}
	errL := fileLoggerFrom(errFile, KindError)
	errorLog.Store(&errL)
	saf := fileLogger(opened[3], KindSafe)
	safeLog.Store(&saf)
	return opened, errFile, nil
}

// fileLogger 构造一路文件日志器。默认同时回显到标准输出；
// 后台子进程关掉回显后只写文件。
func fileLogger(file *os.File, kind string) zerolog.Logger {
	return fileLoggerFrom(file, kind)
}

func fileLoggerFrom(file io.Writer, kind string) zerolog.Logger {
	var out io.Writer = file
	if consoleEcho.Load() {
		out = io.MultiWriter(os.Stdout, file)
	}
	return zerolog.New(&syncWriter{w: out}).
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
