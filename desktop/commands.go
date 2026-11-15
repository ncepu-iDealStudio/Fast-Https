package main

import (
	"fmt"
	"strings"

	"fast-https-desktop/internal/cli"
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

// RunCommand 转交一条 CLI 命令，供界面里的补充面板使用
// （运行快照、Monitor、操作记录、健康检查、证书检查、ACME、配置版本等）。
//
// args 由前端拼好（命令名 + 位置参数 + flag），这里只做基本的空白清理
// 与结果封套转换，**不注入任何额外参数**——不同命令支持的 flag 各不相同
// （operation list 就没有 --config），统一加参数会让命令报
// `unknown flag` 并以退出码 2 失败。配置路径由 CLI 按自己的默认值解析，
// 桌面端的工作目录已经指向程序目录，两者一致。
//
// 不做白名单校验——命令本身就是 CLI 的公开命令树，越权的部分（写操作）
// 由 CLI 自己的 --yes / 配置事务把关。失败时把封套错误翻译成人话
// （cli.FriendlyError），界面不再显示英文错误码或 Go 技术细节。
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
