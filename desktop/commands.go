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

// commandSpec 是前端命令面板用的元数据：一条命令、它的参数与展示信息。
// 清单在这里维护（与 CLI 命令树一一对应），前端只负责渲染表单与结果。
type CommandSpec struct {
	Group    string   `json:"group"`
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Args     []string `json:"args"`
	Flags    []string `json:"flags"`
	NeedsYes bool     `json:"needsYes"`
	TextOnly bool     `json:"textOnly"`
}

// commandSpecs 覆盖 CLI 的全部命令。加命令时只改这张表，不用动前端。
var commandSpecs = []CommandSpec{
	// ---- 站点 ----
	{Group: "站点", Name: "site list", Label: "列出站点", Flags: []string{"--config"}},
	{Group: "站点", Name: "site show", Label: "查看站点生效配置", Flags: []string{"--config", "--merged"}},
	{Group: "站点", Name: "site add", Label: "新增站点", Flags: []string{"--domain", "--listen", "--root", "--proxy-pass", "--tls", "--yes", "--dry-run", "--config"}, NeedsYes: true},
	{Group: "站点", Name: "site update", Label: "更新站点", Flags: []string{"--domain", "--server-name", "--listen", "--root", "--proxy-pass", "--tls", "--yes", "--dry-run", "--config"}, NeedsYes: true},
	{Group: "站点", Name: "site delete", Label: "删除站点", Flags: []string{"--yes", "--dry-run", "--config"}, NeedsYes: true},
	{Group: "站点", Name: "site enable", Label: "启用站点", Flags: []string{"--config"}},
	{Group: "站点", Name: "site disable", Label: "停用站点", Flags: []string{"--config"}},
	// ---- 配置 ----
	{Group: "配置", Name: "config path", Label: "运行期路径", Flags: []string{"--config"}},
	{Group: "配置", Name: "config show", Label: "显示配置（脱敏）", Flags: []string{"--config"}},
	{Group: "配置", Name: "config validate", Label: "校验配置", Flags: []string{"--config", "--candidate"}},
	{Group: "配置", Name: "config versions", Label: "配置版本", Flags: []string{"--config"}},
	{Group: "配置", Name: "config diff", Label: "比较候选配置", Flags: []string{"--candidate", "--config"}},
	{Group: "配置", Name: "config apply", Label: "应用候选配置", Flags: []string{"--candidate", "--yes", "--dry-run", "--config"}, NeedsYes: true},
	{Group: "配置", Name: "config rollback", Label: "回滚版本", Flags: []string{"--version", "--yes", "--dry-run", "--config"}, NeedsYes: true},
	// ---- 服务 ----
	{Group: "服务", Name: "status", Label: "服务运行状态"},
	{Group: "服务", Name: "version", Label: "CLI 版本", TextOnly: true},
	{Group: "服务", Name: "start", Label: "启动服务", TextOnly: true},
	{Group: "服务", Name: "stop", Label: "停止服务", TextOnly: true},
	{Group: "服务", Name: "reload", Label: "重载配置", TextOnly: true},
	{Group: "服务", Name: "service status", Label: "服务状态（详细）"},
	{Group: "服务", Name: "service version", Label: "CLI / 服务版本"},
	{Group: "服务", Name: "service reload", Label: "受控重载"},
	// ---- 运行快照 ----
	{Group: "运行", Name: "runtime status", Label: "当前运行态"},
	{Group: "运行", Name: "runtime stats", Label: "累计统计"},
	{Group: "运行", Name: "diagnose", Label: "健康检查汇总"},
	// ---- Monitor ----
	{Group: "监控", Name: "monitor status", Label: "Monitor 状态"},
	{Group: "监控", Name: "monitor events", Label: "Monitor 事件", Flags: []string{"--limit", "--severity", "--since"}},
	// ---- 证书 ----
	{Group: "证书", Name: "cert list", Label: "证书清单"},
	{Group: "证书", Name: "cert check", Label: "检查域名证书", Flags: []string{"--domain"}},
	{Group: "证书", Name: "acme show", Label: "ACME 设置"},
	// ---- 日志与操作记录 ----
	{Group: "日志", Name: "logs access", Label: "访问日志", Flags: []string{"--limit", "--level", "--keyword"}},
	{Group: "日志", Name: "logs error", Label: "错误日志", Flags: []string{"--limit", "--level", "--keyword"}},
	{Group: "日志", Name: "logs query", Label: "按条件查询日志", Flags: []string{"--kind", "--limit", "--offset", "--level", "--status", "--keyword", "--since", "--until"}},
	{Group: "操作", Name: "operation list", Label: "近期操作", Flags: []string{"--limit"}},
	{Group: "操作", Name: "operation get", Label: "查询一条操作", Flags: []string{"--id"}},
	// ACME setup 是交互式向导，不适合放到通用命令面板里执行
}

// ListCommands 给前端命令面板用。
func (s *DesktopService) ListCommands() []CommandSpec {
	return commandSpecs
}

// RunCommand 转交一条 CLI 命令。
//
// args 由前端拼好（命令名 + 位置参数 + flag），这里只做基本的空白清理和
// 结果封套转换；不做白名单校验——命令本身就是 CLI 的公开命令树，越权的
// 部分（写操作）由 CLI 自己的 --yes / 配置事务把关。
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
			return CommandResult{}, err
		}
		return CommandResult{Command: name, Text: text}, nil
	}

	data, warnings, err := cli.RunJSON[any](s.baseDir, clean...)
	if err != nil {
		return CommandResult{}, err
	}
	return CommandResult{Command: name, Data: data, Warnings: warnings}, nil
}

// isTextOnly 判断哪些命令没有 --json 封套（纯文本输出）。
func isTextOnly(args []string) bool {
	head := strings.Join(args, " ")
	for _, c := range commandSpecs {
		if !c.TextOnly {
			continue
		}
		if head == c.Name || strings.HasPrefix(head, c.Name+" ") {
			return true
		}
	}
	return false
}
