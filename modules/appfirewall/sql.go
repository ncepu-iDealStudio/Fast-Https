package appfirewall

import (
	"fast-https/infra/logging"
	"fast-https/runtime/request"
	"regexp"
)

// sqliPatterns 是保守的 SQL 注入特征：只匹配高置信度的注入语法，
// 尽量避免把正常查询串（日期、编号里的连字符等）误杀。
var sqliPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bunion\s+(all\s+)?select\b`),
	regexp.MustCompile(`(?i);\s*drop\s+table\b`),
	regexp.MustCompile(`(?i);\s*delete\s+from\b`),
	regexp.MustCompile(`(?i)\bor\s+1\s*=\s*1\b`),
	regexp.MustCompile(`(?i)\band\s+1\s*=\s*1\b`),
	regexp.MustCompile(`(?i)\bsleep\s*\(\s*\d`),
	regexp.MustCompile(`(?i)\bbenchmark\s*\(`),
	regexp.MustCompile(`(?i)'\s*or\s*'`),
	regexp.MustCompile(`(?i)\binformation_schema\b`),
	regexp.MustCompile(`(?i)\bload_file\s*\(`),
	regexp.MustCompile(`(?i)\binto\s+(out|dump)file\b`),
}

// HandleSql 对 query 参数与请求体做保守的 SQL 注入检测。
// 此前它只打日志恒放行——配置里写了 appfirewall: ["sql"] 的用户
// 拿到的是零保护；现在检测到注入特征即拒绝。
// 注意：这不是完整的 WAF，只覆盖最常见的注入语法。
func HandleSql(req *request.Request) bool {
	logging.ConsoleLog.Debug().Msg("This is appfirewall, handle sql")

	for key, value := range req.Query {
		if matchAny(sqliPatterns, key) || matchAny(sqliPatterns, value) {
			return false
		}
	}
	if req.Body.Len() > 0 {
		if matchAny(sqliPatterns, req.Body.String()) {
			return false
		}
	}
	return true
}

func matchAny(patterns []*regexp.Regexp, s string) bool {
	for _, p := range patterns {
		if p.MatchString(s) {
			return true
		}
	}
	return false
}
