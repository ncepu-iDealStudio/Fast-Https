// Package cli 是桌面端访问主程序的唯一通道。
//
// 桌面端不直接读写配置、站点、证书或日志文件：所有操作都通过
// `fast-https <command> --json` 转交，由主程序的配置事务负责校验、恢复点、
// reload 与回滚。这样桌面端和命令行看到的永远是同一套语义，也不会绕过
// 事务留下不一致的磁盘状态。
//
// 唯一的例外是窗口/托盘这类纯 UI 行为，以及打开系统文件管理器。
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrorBody 是 --json 模式下失败时的错误封套。
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope 是 --json 模式下 stdout 的唯一文档。
// 日志走 stderr，不会混进来。
type Envelope[T any] struct {
	OK       bool       `json:"ok"`
	Command  string     `json:"command"`
	Data     T          `json:"data"`
	Warnings []string   `json:"warnings"`
	Error    *ErrorBody `json:"error,omitempty"`
}

// Err 把失败封套转成 error。成功但 ok=false、或 ok=true 却带 error 字段，
// 都按失败处理——桌面端不能把这类响应当成操作完成。
func (e *Envelope[T]) Err() error {
	if e.OK && e.Error == nil {
		return nil
	}
	if e.Error != nil {
		msg := e.Error.Message
		if e.Error.Code != "" {
			msg = e.Error.Code + ": " + msg
		}
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("命令返回失败")
}

// Binary 返回与桌面端同目录的主程序路径。
func Binary(dir string) string {
	name := "fast-https"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

func command(dir string, args []string) (*exec.Cmd, error) {
	exe := Binary(dir)
	if _, err := os.Stat(exe); err != nil {
		// 开发态可能只有 go run，允许退回 PATH 里的可执行文件
		if _, pathErr := exec.LookPath(exe); pathErr != nil {
			return nil, fmt.Errorf("未找到主程序 %s：桌面端需要与主程序放在同一目录", exe)
		}
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	return cmd, nil
}

// Run 执行一条命令并返回合并后的文本输出（用于 status 这类纯文本命令）。
func Run(dir string, args ...string) (string, error) {
	cmd, err := command(dir, args)
	if err != nil {
		return "", err
	}
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return "", fmt.Errorf("执行 %s %s 失败：%w", filepath.Base(Binary(dir)), strings.Join(args, " "), err)
	}
	return text, nil
}

// RunBareJSON 执行一条「--json 输出裸 JSON、不套封套」的命令。
// 目前只有 config path（保留桌面端旧契约，直接输出 RuntimePaths）。
func RunBareJSON[T any](dir string, args ...string) (T, error) {
	var zero T
	full := append(append([]string{}, args...), "--json")
	cmd, err := command(dir, full)
	if err != nil {
		return zero, err
	}
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return zero, fmt.Errorf("%s 失败：%s", strings.Join(args, " "), msg)
	}
	var v T
	if err := json.Unmarshal(out, &v); err != nil {
		return zero, fmt.Errorf("解析 %s 输出失败：%w", strings.Join(args, " "), err)
	}
	return v, nil
}

// ---- CLI data 的取值助手 ----
//
// 命令的 data 形状由主程序定义（docs/cli/cli-json-shapes.md），桌面端用
// map + 断言消费，不复制一份结构体。这里提供安全的取值函数，缺失或类型
// 不符都返回零值，不让界面因为一个字段崩掉。

// AsMap 把 data 断言成对象。
func AsMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// AsSlice 把 data 断言成数组。
func AsSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// Str 取字符串字段。
func Str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// Bool 取布尔字段（缺失为 false）。
func Bool(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// Int 取整数字段；JSON 数字可能是 float64，一并兼容。
func Int(m map[string]any, key string) int { return int(Int64(m, key)) }

// Int64 取整数字段；JSON 数字可能是 float64，一并兼容。
func Int64(m map[string]any, key string) int64 {
	switch n := m[key].(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case json.Number:
		v, _ := n.Int64()
		return v
	}
	return 0
}

// Strs 取字符串数组字段。
func Strs(m map[string]any, key string) []string {
	var out []string
	for _, x := range AsSlice(m[key]) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// RunJSON 执行一条命令并解出 --json 的 data。
// args 里不用带 --json，这里统一补上。
func RunJSON[T any](dir string, args ...string) (T, []string, error) {
	var zero T
	full := append([]string{}, args...)
	full = append(full, "--json")
	cmd, err := command(dir, full)
	if err != nil {
		return zero, nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return zero, nil, fmt.Errorf("%s 失败：%s", strings.Join(args, " "), msg)
	}

	var env Envelope[T]
	if err := json.Unmarshal(out, &env); err != nil {
		return zero, nil, fmt.Errorf("解析 %s 输出失败：%w", strings.Join(args, " "), err)
	}
	if err := env.Err(); err != nil {
		return zero, env.Warnings, err
	}
	return env.Data, env.Warnings, nil
}
