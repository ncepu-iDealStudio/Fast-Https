package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// JSON 封套。成功与失败都只往 stdout 写一个文档，诊断信息走 stderr。

type okEnvelope struct {
	OK        bool     `json:"ok"`
	Command   string   `json:"command"`
	RequestID *string  `json:"request_id"`
	Data      any      `json:"data"`
	Warnings  []string `json:"warnings"`
}

type errorBody struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details"`
}

type errEnvelope struct {
	OK        bool      `json:"ok"`
	Command   string    `json:"command"`
	RequestID *string   `json:"request_id"`
	Error     errorBody `json:"error"`
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// JSONMode 报告命令是否带了 --json。
func JSONMode(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func requestIDOf(cmd *cobra.Command) string {
	v, _ := cmd.Flags().GetString("request-id")
	return v
}

// EnsureRequestID 返回命令的请求标识，缺省时生成一个并写回 flag。
func EnsureRequestID(cmd *cobra.Command) string {
	id := strings.TrimSpace(requestIDOf(cmd))
	if id != "" {
		return id
	}
	var b [8]byte
	_, _ = io.ReadFull(rand.Reader, b[:])
	id = "req_" + hex.EncodeToString(b[:])
	_ = cmd.Flags().Set("request-id", id)
	return id
}

// CmdTimeout 取 --timeout，未设置或非正数时用命令自己的默认值。
func CmdTimeout(cmd *cobra.Command, def time.Duration) time.Duration {
	d, _ := cmd.Flags().GetDuration("timeout")
	if d <= 0 {
		return def
	}
	return d
}

// EmitOK 在 --json 模式下输出成功封套。命令自己决定文本模式怎么输出。
func EmitOK(cmd *cobra.Command, id string, data any, warnings []string) error {
	if warnings == nil {
		warnings = []string{}
	}
	return writeJSON(cmd.OutOrStdout(), okEnvelope{
		OK:        true,
		Command:   id,
		RequestID: strPtr(requestIDOf(cmd)),
		Data:      data,
		Warnings:  warnings,
	})
}

// ReportError 把 Execute 返回的错误交给用户，并返回退出码。
// 文本模式与 cobra 原来的打印一致（"Error: ..." 到 stderr）；
// --json 模式下 stdout 是错误封套，stderr 不再重复。
func ReportError(args []string, err error, out, errOut io.Writer) int {
	if !argBool(args, "json") {
		fmt.Fprintln(errOut, "Error:", err)
		return ExitCode(err)
	}

	body := errorBody{Code: CodeInternal, Message: err.Error(), Details: []ErrorDetail{}}
	id := commandIDFromArgs(args)
	var ce *CLIError
	if errors.As(err, &ce) {
		body.Code = ce.Code
		body.Message = ce.Message
		if ce.Details != nil {
			body.Details = ce.Details
		}
		if ce.Command != "" {
			id = ce.Command
		}
	}
	_ = writeJSON(out, errEnvelope{
		OK:        false,
		Command:   id,
		RequestID: strPtr(argString(args, "request-id")),
		Error:     body,
	})
	return ExitCode(err)
}

// commandIDFromArgs 取参数里开头的非 flag 词拼成命令标识。
// 只在命令没有自己给出标识（例如参数解析阶段就失败）时使用。
func commandIDFromArgs(args []string) string {
	var words []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		words = append(words, a)
	}
	if len(words) == 0 {
		return "fast-https"
	}
	return strings.Join(words, ".")
}

// argBool 在原始参数里找布尔 flag。参数解析失败时 cobra 的 flag 值不可靠，
// 所以错误出口直接看命令行。
func argBool(args []string, name string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--"+name {
			return true
		}
		if v, ok := strings.CutPrefix(a, "--"+name+"="); ok {
			return v == "true" || v == "1"
		}
	}
	return false
}

func argString(args []string, name string) string {
	for i, a := range args {
		if a == "--" {
			return ""
		}
		if a == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, "--"+name+"="); ok {
			return v
		}
	}
	return ""
}
