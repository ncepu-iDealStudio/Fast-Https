package main

import (
	"fmt"
	"strings"

	"fast-https-desktop/internal/cli"
	"fast-https-desktop/internal/view"
)

// CommandResult 是一次 CLI 转交的结果。
//
// Data 直接放 --json 封套里的 data，不做中间 DTO：桌面端是 CLI 的图形前端，
// 命令形状由主程序定义，桌面端若再定义一遍结构体，每加一个 CLI 字段就要
// 同步改两处。前端按 command 自己决定怎么渲染。
type CommandResult struct {
	Command  string   `json:"command"`
	Data     any      `json:"data"`
	Warnings []string `json:"warnings"`
	Text     string   `json:"text,omitempty"`
}

// textOnlyCommands 是没有 --json 封套的命令（旧的纯文本命令，以及
// config path、acme show 这两个特例）。这类命令不能补 --json，结果按
// 纯文本返回。
var textOnlyCommands = []string{
	"status", "version", "start", "stop", "reload",
	"config path", "acme show",
}

// needsConfigArg 列出支持 --config 的命令前缀（这些命令要按主程序报出的
// 配置路径找配置树，不能依赖 CLI 进程的 CWD——dev 模式下配置可能被放在
// 任意目录，例如 desktop/bin/config）。
// 排除：旧纯文本命令、config path、acme show（不收 --config）、
// acme setup（用 --config 但由前端显式传）、service/reload。
var needsConfigArg = []string{
	"site list", "site show", "site add", "site update", "site delete", "site enable", "site disable",
	"config show", "config validate", "config versions", "config diff", "config apply", "config rollback",
	"cert list", "cert check",
	"monitor status", "monitor events",
	"logs access", "logs error", "logs query",
	"diagnose", "runtime status", "runtime stats",
	"operation list", "operation get",
}

func shouldAddConfig(args []string) bool {
	if len(args) == 0 {
		return false
	}
	// 已显式带 --config 就不再补（尊重调用方）
	for _, a := range args {
		if strings.HasPrefix(a, "--config") {
			return false
		}
	}
	head := strings.Join(args, " ")
	for _, p := range needsConfigArg {
		if head == p || strings.HasPrefix(head, p+" ") {
			return true
		}
	}
	return false
}

// RunCommand 转交一条 CLI 命令，供界面里的补充面板使用
// （运行快照、Monitor、操作记录、健康检查、证书检查、ACME、配置版本等）。
//
// args 由前端拼好（命令名 + 位置参数 + flag），这里只做基本的空白清理、
// 自动补 --config、结果封套转换；不做白名单校验——命令本身就是 CLI 的
// 公开命令树，越权的部分（写操作）由 CLI 自己的 --yes / 配置事务把关。
// 失败时把封套错误翻译成人话（cli.FriendlyError），界面不再显示英文
// 错误码或 Go 技术细节。
func (s *DesktopService) RunCommand(args []string) (CommandResult, error) {
	clean := make([]string, 0, len(args))
	for _, a := range args {
		if v := strings.TrimSpace(a); v != "" {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return CommandResult{}, fmt.Errorf("命令为空")
	}
	name := strings.Join(clean, " ")

	if shouldAddConfig(clean) {
		if p := view.ResolvePaths(s.baseDir).Config; p != "" {
			clean = append(clean, "--config", p)
		}
	}

	if isTextOnly(clean) {
		text, err := cli.Run(s.baseDir, clean...)
		if err != nil {
			return CommandResult{}, fmt.Errorf("%s", cli.FriendlyError(err, name))
		}
		return CommandResult{Command: name, Text: text}, nil
	}

	data, warnings, err := cli.RunJSON[any](s.baseDir, clean...)
	if err != nil {
		return CommandResult{}, fmt.Errorf("%s", cli.FriendlyError(err, name))
	}
	return CommandResult{Command: name, Data: data, Warnings: warnings}, nil
}

// isTextOnly 判断哪些命令没有 --json 封套（纯文本输出）。
func isTextOnly(args []string) bool {
	head := strings.Join(args, " ")
	for _, name := range textOnlyCommands {
		if head == name || strings.HasPrefix(head, name+" ") {
			return true
		}
	}
	return false
}
