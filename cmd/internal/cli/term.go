package cli

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

// ApplyStdio 把表单的输入输出接到指定流上，并在不适合全屏渲染时降级为
// 可访问模式（逐行问答）。
//
// accessible 为 true、或标准输出不是终端（被重定向 / 被 GUI 捕获）时都会
// 降级；后者是必要的，否则重定向场景会卡在原地等按键。
func ApplyStdio(form *huh.Form, in io.Reader, out io.Writer, accessible bool) {
	if in != nil {
		form.WithInput(in)
	}
	if out != nil {
		form.WithOutput(out)
	}
	if accessible || !term.IsTerminal(os.Stdout.Fd()) {
		form.WithAccessible(true).WithShowHelp(false)
	}
}

// InteractiveAvailable 只看标准输入：输入是终端就能问问题，
// 输出被重定向时由 ApplyStdio 降级成逐行问答。
func InteractiveAvailable() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

// ValidateNotBlank 是向导里通用的非空校验。
func ValidateNotBlank(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("不能为空")
	}
	return nil
}
