package config

import (
	"fast-https/infra/logging"
	"fmt"
	"strconv"
	"strings"
)

// ACME 挑战名、默认 CA 目录与默认值。
const (
	// ACMEChallengeAuto 表示由进程按可用性自动排序挑战，默认值。
	ACMEChallengeAuto      = "auto"
	ACMEChallengeHTTP01    = "http-01"
	ACMEChallengeTLSALPN01 = "tls-alpn-01"

	// 默认降级链：先 Let's Encrypt，失败再 ZeroSSL，全部失败回落本地自签。
	ACMELetsEncryptDirectory        = "https://acme-v02.api.letsencrypt.org/directory"
	ACMELetsEncryptStagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ACMEZeroSSLDirectory            = "https://acme.zerossl.com/v2/DV90"

	DefaultACMERenewBeforeDays = 30

	// ACMECertStorageDir 相对 FAST_HTTPS_BASE_DIR，与 CERT_DIR 同层但不冲突。
	ACMECertStorageDir = "config/cert/acme"
)

// ACMECA 是降级链中的一个 CA。ZeroSSL 这类 CA 要求账户绑定 EAB，
// EAB 缺失时该 CA 会被跳过，不做匿名注册尝试。
type ACMECA struct {
	Name       string `json:"name"`
	Directory  string `json:"directory"`
	EABKid     string `json:"eab_kid"`
	EABHmacKey string `json:"eab_hmac_key"`
}

// AcmeConfig 是顶层 acme 段。缺省时 enabled=false，行为与 1.4.0 一致。
type AcmeConfig struct {
	Enabled   bool     `json:"enabled"`
	Email     string   `json:"email"`
	Staging   bool     `json:"staging"`
	Challenge []string `json:"challenge"`
	// RenewBeforeDays 证书到期前的续期提前量，0 表示用默认 30 天。
	RenewBeforeDays int `json:"renew_before_days"`
	// FallbackSelfSigned 用指针区分「没写」与「显式 false」：默认回落自签。
	FallbackSelfSigned *bool    `json:"fallback_self_signed"`
	Storage            string   `json:"storage"`
	CAs                []ACMECA `json:"cas"`
}

// DefaultACMECAs 返回默认 CA 降级链。
func DefaultACMECAs() []ACMECA {
	return []ACMECA{
		{Name: "letsencrypt", Directory: ACMELetsEncryptDirectory},
		{Name: "zerossl", Directory: ACMEZeroSSLDirectory},
	}
}

// FallbackSelfSignedEnabled 报告是否允许签发失败时继续用自签证书。
func (c *AcmeConfig) FallbackSelfSignedEnabled() bool {
	if c == nil || c.FallbackSelfSigned == nil {
		return true
	}
	return *c.FallbackSelfSigned
}

// Active 报告 acme 段是否启用。缺省段（nil）视为关闭。
func (c *AcmeConfig) Active() bool {
	return c != nil && c.Enabled
}

// UsesChallenge 报告配置的挑战列表是否可能落到指定挑战。
// auto 表示由进程按可用性排序，因此对任一挑战都返回真。
func (c *AcmeConfig) UsesChallenge(name string) bool {
	if c == nil {
		return false
	}
	for _, v := range c.Challenge {
		if v == name || v == ACMEChallengeAuto {
			return true
		}
	}
	return false
}

// warnHTTP01WithoutPort80 在 http-01 可能被使用却没有任何 server 监听 80 时告警。
// 前面可能有反向代理转发 /.well-known/acme-challenge，所以只警告不报错。
func (c *AcmeConfig) warnHTTP01WithoutPort80(servers []jsonServer) {
	if !c.Active() || !c.UsesChallenge(ACMEChallengeHTTP01) {
		return
	}
	for i := range servers {
		for _, listen := range servers[i].Listen {
			if listenPortOf(listen) == 80 {
				return
			}
		}
	}
	logging.ConsoleLog.Warn().Msg("acme: http-01 is enabled but no http.server listens on port 80; " +
		"assuming a front proxy forwards /.well-known/acme-challenge to this process")
}

func listenPortOf(listen string) int {
	fields := strings.Fields(strings.TrimSpace(listen))
	if len(fields) == 0 {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimPrefix(fields[0], ":"))
	if err != nil {
		return 0
	}
	return port
}

// applyDefaults 补齐缺省值，并把存储目录转成绝对路径。
// 只在解析配置后调用一次，不在校验路径上调用。
func (c *AcmeConfig) applyDefaults() {
	if c == nil {
		return
	}
	c.Email = strings.TrimSpace(c.Email)
	if len(c.Challenge) == 0 {
		c.Challenge = []string{ACMEChallengeAuto}
	}
	if c.RenewBeforeDays == 0 {
		c.RenewBeforeDays = DefaultACMERenewBeforeDays
	}
	if c.FallbackSelfSigned == nil {
		v := true
		c.FallbackSelfSigned = &v
	}
	if strings.TrimSpace(c.Storage) == "" {
		c.Storage = ACMECertStorageDir
	}
	c.Storage = absConfigPath(c.Storage)
	if len(c.CAs) == 0 {
		c.CAs = DefaultACMECAs()
	}
	if c.Staging {
		// staging 只替换 Let's Encrypt 的生产目录，用户自定义的目录保持原样。
		for i := range c.CAs {
			if c.CAs[i].Directory == ACMELetsEncryptDirectory {
				c.CAs[i].Directory = ACMELetsEncryptStagingDirectory
			}
		}
	}
}

// validate 校验 acme 段。未启用时仍校验枚举值，避免把错别字留到启用那天。
func (c *AcmeConfig) validate() error {
	if c == nil {
		return nil
	}
	if err := c.validateChallenge(); err != nil {
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

func (c *AcmeConfig) validateChallenge() error {
	if len(c.Challenge) == 0 {
		return nil
	}
	hasAuto := false
	for i, v := range c.Challenge {
		switch v {
		case ACMEChallengeAuto:
			hasAuto = true
		case ACMEChallengeHTTP01, ACMEChallengeTLSALPN01:
		default:
			return fmt.Errorf("acme.challenge[%d] is invalid: %s (want auto, http-01 or tls-alpn-01)", i, v)
		}
	}
	if hasAuto && len(c.Challenge) > 1 {
		return fmt.Errorf("acme.challenge: auto cannot be combined with other values")
	}
	return nil
}
