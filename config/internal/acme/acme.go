// Package acme 是 acme 段的配置行为：默认值、校验、http-01 告警与写回。
// 类型定义在 model 包，证书实际签发在 modules/certmanager。
package acme

import (
	"fmt"
	"strings"

	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/schema"
)

// DefaultACMECAs 返回默认 CA 降级链。
func DefaultACMECAs() []model.ACMECA {
	return []model.ACMECA{
		{Name: "letsencrypt", Directory: model.ACMELetsEncryptDirectory},
		{Name: "zerossl", Directory: model.ACMEZeroSSLDirectory},
	}
}

// HTTP01Port80Warning 在 http-01 可能被使用却没有任何 server 监听 80 时返回告警文本，
// 否则返回空串。前面可能有反向代理转发 /.well-known/acme-challenge，所以只警告不报错。
func HTTP01Port80Warning(c *model.AcmeConfig, servers []schema.Server) string {
	if !c.Active() || !c.UsesChallenge(model.ACMEChallengeHTTP01) {
		return ""
	}
	for i := range servers {
		for _, listen := range servers[i].Listen {
			if schema.ListenPort(listen) == 80 {
				return ""
			}
		}
	}
	return "acme: http-01 is enabled but no http.server listens on port 80; " +
		"assuming a front proxy forwards /.well-known/acme-challenge to this process"
}

// ApplyDefaults 补齐缺省值，并把存储目录转成绝对路径。
// 只在解析配置后调用一次，不在校验路径上调用。
func ApplyDefaults(c *model.AcmeConfig) {
	if c == nil {
		return
	}
	c.Email = strings.TrimSpace(c.Email)
	if len(c.Challenge) == 0 {
		c.Challenge = []string{model.ACMEChallengeAuto}
	}
	if c.RenewBeforeDays == 0 {
		c.RenewBeforeDays = model.DefaultACMERenewBeforeDays
	}
	if c.FallbackSelfSigned == nil {
		v := true
		c.FallbackSelfSigned = &v
	}
	if strings.TrimSpace(c.Storage) == "" {
		c.Storage = model.ACMECertStorageDir
	}
	c.Storage = paths.ETCPath(c.Storage)
	if len(c.CAs) == 0 {
		c.CAs = DefaultACMECAs()
	}
	if c.Staging {
		// staging 只替换 Let's Encrypt 的生产目录，用户自定义的目录保持原样。
		for i := range c.CAs {
			if c.CAs[i].Directory == model.ACMELetsEncryptDirectory {
				c.CAs[i].Directory = model.ACMELetsEncryptStagingDirectory
			}
		}
	}
}

// Validate 校验 acme 段。未启用时仍校验枚举值，避免把错别字留到启用那天。
func Validate(c *model.AcmeConfig) error {
	if c == nil {
		return nil
	}
	if err := validateChallenge(c); err != nil {
		return err
	}
	if c.RenewBeforeDays < 0 {
		return fmt.Errorf("acme.renew_before_days must not be negative: %d", c.RenewBeforeDays)
	}
	seen := make(map[string]bool, len(c.CAs))
	for i, ca := range c.CAs {
		if strings.TrimSpace(ca.Name) == "" {
			return fmt.Errorf("acme.cas[%d].name is required", i)
		}
		if strings.TrimSpace(ca.Directory) == "" {
			return fmt.Errorf("acme.cas[%d].directory is required", i)
		}
		if seen[ca.Name] {
			return fmt.Errorf("acme.cas[%d].name is duplicated: %s", i, ca.Name)
		}
		seen[ca.Name] = true
	}
	if c.Enabled && strings.TrimSpace(c.Email) == "" {
		return fmt.Errorf("acme.email is required when acme.enabled is true")
	}
	return nil
}

func validateChallenge(c *model.AcmeConfig) error {
	if len(c.Challenge) == 0 {
		return nil
	}
	hasAuto := false
	for i, v := range c.Challenge {
		switch v {
		case model.ACMEChallengeAuto:
			hasAuto = true
		case model.ACMEChallengeHTTP01, model.ACMEChallengeTLSALPN01:
		default:
			return fmt.Errorf("acme.challenge[%d] is invalid: %s (want auto, http-01 or tls-alpn-01)", i, v)
		}
	}
	if hasAuto && len(c.Challenge) > 1 {
		return fmt.Errorf("acme.challenge: auto cannot be combined with other values")
	}
	return nil
}
