// waf.go 封装 GuardianWAF 引擎：构建、原子切换与全项目唯一的检测入口。
//
// 边界约束（tests/arch 守卫）：guardianwaf 只允许出现在本包与 cmd，
// core/runtime 不感知检测引擎的存在。
package appfirewall

import (
	"strings"
	"sync/atomic"

	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/dynlog"
	"fast-https/runtime/listener"
	"fast-https/runtime/response"

	"github.com/guardianwaf/guardianwaf"
)

// guardianWAF 版本钉死 v1.0.0：检测器名与模式常量以该版本源码为准
// （检测器：sqli/xss/lfi/cmdi/xxe/ssrf；模式：enforce/monitor/disabled）。

type wafEngine struct {
	e *guardianwaf.Engine
}

// current 持有当前引擎；nil = 未启用 = 所有请求放行（零开销默认）。
var current atomic.Pointer[wafEngine]

// wafDetectorNames 是 v1.0.0 的全部检测器（WithDetector 合法名）。
var wafDetectorNames = []string{"sqli", "xss", "lfi", "cmdi", "xxe", "ssrf"}

// legacyRuleMap 把旧 appfirewall 规则名映射到检测器名。
var legacyRuleMap = map[string]string{
	"sql": "sqli",
	"xss": "xss",
}

// validWAFMode 判定 waf.mode 配置是否合法。
func validWAFMode(mode string) bool {
	switch mode {
	case "", guardianwaf.ModeEnforce, guardianwaf.ModeMonitor, guardianwaf.ModeDisabled:
		return true
	}
	return false
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
// 返回 (检测器名集合, 是否启用)。
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

// BuildWAF 按配置快照构建检测引擎。
// 由 AfterListen 钩子在监听器创建后与每次 reload 时调用；
// 未配置防火墙时不构建引擎（current 保持 nil，零开销）。
func BuildWAF(cfg *config.Fast_Https) error {
	old := current.Load()

	if !wafConfigured(cfg) {
		current.Store(nil)
		closeEngine(old)
		return nil
	}

	mode := guardianwaf.ModeEnforce
	if cfg.Waf != nil && cfg.Waf.Mode != "" {
		mode = cfg.Waf.Mode
	}
	if mode == guardianwaf.ModeDisabled {
		current.Store(nil)
		closeEngine(old)
		return nil
	}

	detectors := resolveDetectors(cfg)
	opts := []guardianwaf.Option{guardianwaf.WithMode(mode)}
	for _, name := range wafDetectorNames {
		if detectors[name] {
			opts = append(opts, guardianwaf.WithDetector(name, true, 1.0))
		} else {
			opts = append(opts, guardianwaf.WithDetector(name, false, 1.0))
		}
	}

	e, err := guardianwaf.New(guardianwaf.Config{}, opts...)
	if err != nil {
		// 构建失败保留旧引擎继续服务
		logging.ConsoleLog.Error().Msgf("waf: build failed, keeping previous engine: %s", err.Error())
		return err
	}
	current.Store(&wafEngine{e: e})
	closeEngine(old)
	logging.ConsoleLog.Info().Msgf("waf: engine started in %s mode with %d detector(s)", mode, len(detectors))
	return nil
}

// ShutdownWAF 释放当前引擎（AfterStop 钩子）。
func ShutdownWAF() {
	closeEngine(current.Swap(nil))
}

func closeEngine(w *wafEngine) {
	if w != nil {
		_ = w.e.Close()
	}
}

// Allowed 是全项目唯一的防火墙检测入口，在 location 匹配与认证之后
// 每请求恰好调用一次。检测命中即写 403 并关闭，返回 false。
// 引擎为 nil（未配置）时恒放行。
func Allowed(_ *listener.ListenCfg, ev *core.Event) bool {
	w := current.Load()
	if w == nil {
		return true
	}
	logging.ConsoleLog.Info().Msg("waf: CHECKING with engine")

	result := w.e.Check(toHTTPRequest(ev))
	if result.Logged && len(result.Findings) > 0 {
		logging.Safe().Debug().
			Str(logging.FieldRemoteAddr, remoteIP(ev)).
			Str(logging.FieldReason, "waf").
			Int("score", result.TotalScore).
			Str("findings", findingsSummary(result)).
			Msg("waf detection logged request")
	}
	if !result.Blocked {
		return true
	}

	logging.Safe().Warn().
		Str(logging.FieldRemoteAddr, remoteIP(ev)).
		Str(logging.FieldReason, "waf").
		Int("score", result.TotalScore).
		Str("action", result.Action).
		Str("detectors", findingsSummary(result)).
		Msg("request blocked by waf")

	ev.RR.Res = response.DefaultForbidden()
	log := dynlog.DynLogger{}
	log.Status(403)
	log.Size(len(ev.RR.Res.Body))
	dynlog.Log(&log, ev)
	ev.WriteResponseClose(nil)
	return false
}

func findingsSummary(r guardianwaf.Result) string {
	names := make([]string, 0, len(r.Findings))
	for _, f := range r.Findings {
		names = append(names, f.Detector)
	}
	return strings.Join(names, ",")
}

func remoteIP(ev *core.Event) string {
	addr := ev.Conn.RemoteAddr().String()
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}
