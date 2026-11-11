// Package cli 的友好错误翻译：把 CLI 封套里的英文错误码/Go 技术细节
// 换成人话，桌面端所有面板（运行快照、Monitor、站点、日志、操作记录）
// 通过 RunCommand 统一受益，不用每个界面各自解析。
package cli

import "strings"

// FriendlyError 把 CLI 错误翻译成中英双语里的一句人话。
// cmd 是命令名（如 "runtime status"），err 的原始文本来自封套 error.message
// 或命令的 stderr。命中不了具体规则时，返回去掉技术细节的原文。
func FriendlyError(err error, cmd string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	return FriendlyMessage(msg, cmd)
}

// FriendlyMessage 是 FriendlyError 的字符串版本，便于单测直接喂样本。
func FriendlyMessage(msg, cmd string) string {
	m := strings.ToLower(msg)
	c := strings.ToLower(cmd)

	// 运行快照：端点没起来是服务未启动的正常表现，不是故障
	if strings.Contains(m, "service_unreachable") ||
		strings.Contains(m, "runtime snapshot unreachable") ||
		strings.Contains(m, "9088") ||
		strings.Contains(m, "connection refused") ||
		strings.Contains(m, "actively refused") ||
		strings.Contains(m, "dial tcp") && strings.Contains(c, "runtime") {
		return "运行快照暂不可用：服务未启动或 /runtime 端点未就绪。启动服务后点「刷新」重试。\n" +
			"Runtime snapshot unavailable: the service is not running or the /runtime endpoint is not ready. Start the service, then click Refresh."
	}

	// 站点停用唯一站点：候选树里没有活动站点，事务校验会拒绝
	if strings.Contains(m, "no server configured") ||
		strings.Contains(m, "add http.server or a server file") {
		return "配置里没有可用站点了：这通常是「停用唯一站点」造成的，全部停用会让配置校验不通过。" +
			"请先添加另一个站点，或改用删除该站点配置。\n" +
			"No usable site remains: disabling the only site makes the whole config invalid. Add another site first, or delete this site instead."
	}
	if strings.Contains(m, "already enabled") {
		return "该站点已经是启用状态，无需重复操作。\nThis site is already enabled."
	}
	if strings.Contains(m, "already disabled") {
		return "该站点已经是停用状态，无需重复操作。\nThis site is already disabled."
	}
	if strings.Contains(m, "already exists") {
		return "同名站点或域名已存在，请换一个名称。\nA site with that name or domain already exists; please choose another."
	}
	if strings.Contains(m, "site conflict") || strings.Contains(m, "conflict") {
		return "操作与当前状态冲突，请刷新列表后再试。\nThe operation conflicts with the current state; refresh the list and try again."
	}
	if strings.Contains(m, "site not found") || strings.Contains(m, "not_found") {
		return "找不到该站点，可能已被删除或改名，请刷新列表。\nSite not found (deleted or renamed); refresh the list."
	}
	if strings.Contains(m, "not_supported") || strings.Contains(m, "inline in the main config") {
		return "该站点内联在主配置 http.server[] 里，不支持在这里操作，请到设置页修改。\n" +
			"This site is inline in http.server[]; edit it on the Settings page instead."
	}
	if strings.Contains(m, "config_invalid") || strings.Contains(m, "config validation") {
		return "配置校验未通过，改动没有被应用。详情：" + clip(msg, 180) + "\n" +
			"Config validation failed; nothing was applied."
	}
	if strings.Contains(m, "log_source_unavailable") || strings.Contains(m, "log file not found") {
		return "日志文件还不存在：服务启动并产生日志后就能查到。\n" +
			"No log file yet; it appears once the service has served requests."
	}
	if strings.Contains(m, "permission") || strings.Contains(m, "access is denied") {
		return "权限不足：请以管理员身份运行桌面端，或检查安装目录是否可写。\n" +
			"Permission denied: run the console as administrator or check the install folder."
	}
	if strings.Contains(m, "not_installed") {
		return "Monitor 未安装或未启动：状态文件还不存在。启动 Monitor 后即可查看。\n" +
			"Monitor is not installed or not running (no status file yet)."
	}
	if strings.Contains(m, "timeout") || strings.Contains(m, "deadline") {
		return "操作超时，请稍后重试。\nThe operation timed out; please try again."
	}
	if strings.Contains(m, "unmarshal") || strings.Contains(m, "invalid json") {
		return "命令返回的数据格式异常，请检查主程序版本是否与桌面端匹配。\n" +
			"Unexpected data from the CLI; check that the service and console versions match."
	}
	return clip(msg, 260)
}

// clip 截断长文本，避免把一整段 Go 错误/JSON 甩给用户。
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
