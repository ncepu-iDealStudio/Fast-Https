// mapping.go 把配置（location 的 appfirewall 规则名、全局 waf 块）
// 映射为 GuardianWAF 的检测器选项。
package appfirewall

import (
	"fast-https/config"
	"fast-https/infra/logging"
)

// wafDetectorNames 是 v1.0.0 的全部检测器（WithDetector 合法名）。
var wafDetectorNames = []string{"sqli", "xss", "lfi", "cmdi", "xxe", "ssrf"}

// legacyRuleMap 把旧 appfirewall 规则名映射到检测器名。
var legacyRuleMap = map[string]string{
	"sql": "sqli",
	"xss": "xss",
}

// wafConfigured 报告是否有任何 location 或全局块启用了防火墙。
func wafConfigured(cfg *config.Fast_Https) bool {
	if cfg.Waf != nil {
		return true
	}
	for i := range cfg.Servers {
		for j := range cfg.Servers[i].Path {
			if len(cfg.Servers[i].Path[j].AppFireWall) > 0 {
				return true
			}
		}
	}
	return false
}

// resolveDetectors 计算启用的检测器集合：
// location 的 appfirewall 名（含旧规则名映射）与直写检测器名取并集；
// 全局 waf 块存在但没有 location 级配置时启用全部检测器。
func resolveDetectors(cfg *config.Fast_Https) map[string]bool {
	set := make(map[string]bool)
	for i := range cfg.Servers {
		for j := range cfg.Servers[i].Path {
			for _, rule := range cfg.Servers[i].Path[j].AppFireWall {
				name, ok := legacyRuleMap[rule]
				if !ok {
					if isKnownDetector(rule) {
						name = rule
					} else {
						logging.ConsoleLog.Warn().Msgf("appfirewall: unknown rule %q is ignored", rule)
						continue
					}
				}
				set[name] = true
			}
		}
	}
	if len(set) == 0 && cfg.Waf != nil {
		// 显式 waf 块 = 全量启用
		for _, name := range wafDetectorNames {
			set[name] = true
		}
	}
	return set
}

func isKnownDetector(name string) bool {
	for _, d := range wafDetectorNames {
		if d == name {
			return true
		}
	}
	return false
}
