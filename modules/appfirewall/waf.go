// waf.go 封装 GuardianWAF 引擎：构建、原子切换与全项目唯一的检测入口。
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
// 检测器选项的配置映射见 mapping.go。

type wafEngine struct {
	e *guardianwaf.Engine
}

// current 持有当前引擎；nil = 未启用 = 所有请求放行（零开销默认）。
var current atomic.Pointer[wafEngine]

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
