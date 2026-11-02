package config

import "fmt"

// Host 校验的三态。默认 off：保持"Host 不匹配就回落到该端口第一个 server"
// 的既有行为，避免用 IP 或别名访问的老站点突然 421。
const (
	HostCheckOff     = "off"
	HostCheckWarn    = "warn"
	HostCheckEnforce = "enforce"
)

// SecurityHeaders 是统一在响应出口写入的安全响应头。
// 除 nosniff 外全部需要显式开启：HSTS 误开会让 HTTP 站点被浏览器锁死。
type SecurityHeaders struct {
	NoSniff        *bool  `json:"nosniff"`
	HSTS           string `json:"hsts"`
	FrameOptions   string `json:"x_frame_options"`
	ReferrerPolicy string `json:"referrer_policy"`
	CSP            string `json:"content_security_policy"`
}

// SecurityConfig 是顶层 security 段。
type SecurityConfig struct {
	HostCheck string          `json:"host_check"`
	Headers   SecurityHeaders `json:"headers"`
}

// applyDefaults 补齐默认值：host_check=off，headers.nosniff=true。
// 与 acme 段一样，只在解析完成后调用一次，校验路径不补默认值。
func (c *SecurityConfig) applyDefaults() {
	if c == nil {
		return
	}
	if c.HostCheck == "" {
		c.HostCheck = HostCheckOff
	}
	if c.Headers.NoSniff == nil {
		enabled := true
		c.Headers.NoSniff = &enabled
	}
}

func (c *SecurityConfig) validate() error {
	if c == nil {
		return nil
	}
	switch c.HostCheck {
	case "", HostCheckOff, HostCheckWarn, HostCheckEnforce:
	default:
		return fmt.Errorf("security.host_check must be one of off/warn/enforce: %q", c.HostCheck)
	}
	return nil
}

// HostCheckMode 返回生效的 Host 校验模式，未配置时是 off。
func (c *SecurityConfig) HostCheckMode() string {
	if c == nil || c.HostCheck == "" {
		return HostCheckOff
	}
	return c.HostCheck
}

// NoSniffEnabled 报告是否写入 X-Content-Type-Options: nosniff。
// 未配置 security 段时也是 true：这一项无兼容风险。
func (c *SecurityConfig) NoSniffEnabled() bool {
	if c == nil || c.Headers.NoSniff == nil {
		return true
	}
	return *c.Headers.NoSniff
}
