// Package certmanager 向 ACME CA 申请并续期证书。
//
// 分工：账户、订单、续期调度、证书落盘在控制面（certmagic + acmez），
// 挑战应答通过数据面钩子完成 —— http-01 走 core.Services.HandleEarlyRequest，
// tls-alpn-01 走 runtime/listener 的挑战证书表。这样控制面不必自己监听
// 80/443，也不会与数据面的端口管理、证书热替换冲突。
package certmanager

import (
	"context"
	"crypto/tls"
	"errors"
	"fast-https/config"
	"fast-https/core"
	"fast-https/infra/files"
	"fast-https/infra/logging"
	"fast-https/runtime/listener"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez/v3"
	"github.com/mholt/acmez/v3/acme"
	"go.uber.org/zap"
)

const acmeUserAgent = "fast-https/" + config.CURRENT_VERSION

// renewalCertLifetime 是 CA 通常给出的证书有效期，用于把
// renew_before_days 换算成 certmagic 的 RenewalWindowRatio。
const renewalCertLifetime = 90 * 24 * time.Hour

// Manager 持有一次 ACME 运行期的全部状态。
type Manager struct {
	cfg   *config.AcmeConfig
	magic *certmagic.Config
	cache *certmagic.Cache
	http  *http01Solver
	alpn  *tlsalpn01Solver

	cancel context.CancelFunc
	once   sync.Once
}

var (
	activeManager atomic.Pointer[Manager]
	startMu       sync.Mutex
)

// Wire 把数据面钩子接到核心服务上。未启用 ACME 时什么都不挂，
// 热路径上只多一次判空。
func Wire(services *core.Services) {
	cfg := config.Current().Acme
	if !cfg.Active() {
		return
	}
	services.HandleEarlyRequest = handleEarlyRequest
	// 只有配置里启用了 tls-alpn-01 才通告 acme-tls/1 并注入挑战证书来源。
	if cfg.UsesChallenge(config.ACMEChallengeTLSALPN01) {
		listener.RegisterTLSALPNProvider(tlsALPNChallengeCert)
	}
}

func handleEarlyRequest(ev *core.Event) bool {
	m := activeManager.Load()
	if m == nil || m.http == nil {
		return false
	}
	return m.http.handle(ev)
}

func tlsALPNChallengeCert(hello *tls.ClientHelloInfo) (*tls.Certificate, bool) {
	m := activeManager.Load()
	if m == nil || m.alpn == nil {
		return nil, false
	}
	return m.alpn.certificate(hello)
}

// Start 在后台为每个 ssl server_name 申请证书。首次握手不会被阻塞：
// 申请完成前继续使用自签证书。可重复调用（reload 后 AfterListen 会再进），
// 只有第一次生效。
func Start() {
	cfg := config.Current().Acme
	if !cfg.Active() {
		return
	}

	startMu.Lock()
	defer startMu.Unlock()
	if activeManager.Load() != nil {
		return
	}

	names := managedDomains(config.Current())
	if len(names) == 0 {
		logging.ConsoleLog.Warn().Msg("acme: no server_name can use ACME, keeping self-signed certificates")
		return
	}

	m, err := newManager(cfg, names)
	if err != nil {
		logging.ConsoleLog.Error().Msgf("acme: disabled, keeping self-signed certificates: %v", err)
		return
	}
	activeManager.Store(m)
	m.run(names)
	logging.ConsoleLog.Info().Msgf("acme: background issuance started for %s", strings.Join(names, ", "))
}

// Stop 停止后台申请与续期。
func Stop() {
	if m := activeManager.Swap(nil); m != nil {
		m.stop()
	}
}

// stop 幂等：取消后台上下文并停掉证书缓存。
func (m *Manager) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.once.Do(func() {
		if m.cache != nil {
			m.cache.Stop()
		}
	})
}

func newManager(cfg *config.AcmeConfig, names []string) (*Manager, error) {
	if err := os.MkdirAll(cfg.Storage, 0o755); err != nil {
		return nil, fmt.Errorf("create acme storage %s: %w", cfg.Storage, err)
	}

	m := &Manager{cfg: cfg}
	solvers := make(map[string]acmez.Solver, 2)
	if cfg.UsesChallenge(config.ACMEChallengeHTTP01) {
		m.http = newHTTP01Solver()
		solvers[acme.ChallengeTypeHTTP01] = m.http
	}
	if cfg.UsesChallenge(config.ACMEChallengeTLSALPN01) {
		m.alpn = newTLSALPN01Solver()
		solvers[acme.ChallengeTypeTLSALPN01] = m.alpn
	}
	if len(solvers) == 0 {
		return nil, errors.New("acme.challenge enables no supported challenge type")
	}

	logger := newACMELogger()
	issuers := make([]certmagic.Issuer, 0, len(cfg.CAs))
	for _, ca := range cfg.CAs {
		if requiresEAB(ca) && (ca.EABKid == "" || ca.EABHmacKey == "") {
			logging.ConsoleLog.Warn().Msgf("acme: skipping CA %s (%s), it requires eab_kid and eab_hmac_key",
				ca.Name, ca.Directory)
			continue
		}
		issuers = append(issuers, newCAIssuer(ca, cfg.Email, cfg.Storage, solvers, logger))
	}
	if len(issuers) == 0 {
		return nil, errors.New("acme.cas has no usable CA")
	}

	var magicPtr atomic.Pointer[certmagic.Config]
	m.cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(certmagic.Certificate) (*certmagic.Config, error) {
			current := magicPtr.Load()
			if current == nil {
				return nil, errors.New("acme: certificate config is not ready")
			}
			return current, nil
		},
		Logger: zap.NewNop(),
	})
	m.magic = certmagic.New(m.cache, certmagic.Config{
		Storage:            &certmagic.FileStorage{Path: cfg.Storage},
		Issuers:            issuers,
		IssuerPolicy:       certmagic.UseFirstIssuer,
		RenewalWindowRatio: renewalWindowRatio(cfg.RenewBeforeDays),
		OnEvent:            m.onEvent,
		Logger:             zap.NewNop(),
	})
	magicPtr.Store(m.magic)

	return m, nil
}

// run 先同步存储里已有的证书（进程重启后不必等续期），再交给
// certmagic 的后台任务负责首次申请与到期前续期。
func (m *Manager) run(names []string) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	go func() {
		m.syncStored(ctx, names)
		if err := m.magic.ManageAsync(ctx, names); err != nil {
			logging.ConsoleLog.Error().Msgf("acme: background management failed: %v", err)
		}
	}()
}

// syncStored 把上次运行留下的证书同步到数据面。存储里有证书但没有
// 覆盖 PEM 的情况（例如换了配置目录）也能靠它恢复。
func (m *Manager) syncStored(ctx context.Context, names []string) {
	for _, name := range names {
		cert, err := m.magic.CacheManagedCertificate(ctx, name)
		if err != nil {
			logging.ConsoleLog.Debug().Msgf("acme: no stored certificate for %s yet", name)
			continue
		}
		certPEM, keyPEM, err := pemPair(cert.Certificate.Certificate, cert.PrivateKey)
		if err != nil {
			logging.ConsoleLog.Error().Msgf("acme: encode stored certificate for %s: %v", name, err)
			continue
		}
		if err := m.commit(name, certPEM, keyPEM); err != nil {
			logging.ConsoleLog.Error().Msgf("acme: apply stored certificate for %s: %v", name, err)
		}
	}
}

// onEvent 是 certmagic 的生命周期回调。在事件里只处理与落盘、日志相关的
// 部分，任何失败都不阻断 certmagic 自己的重试。
func (m *Manager) onEvent(ctx context.Context, event string, data map[string]any) error {
	name, _ := data["identifier"].(string)
	switch event {
	case "cert_obtaining":
		logging.ConsoleLog.Info().Msgf("acme: requesting certificate for %s", name)
	case "cert_obtained":
		if err := m.applyIssued(ctx, name, data); err != nil {
			logging.ConsoleLog.Error().Msgf("acme: certificate for %s was issued but not applied: %v", name, err)
		}
	case "cert_failed":
		// 回落自签：现有证书保持不变，certmagic 会按退避重试。
		logging.ConsoleLog.Error().Msgf("acme: certificate for %s could not be obtained, keeping the current certificate: %v",
			name, data["error"])
	}
	return nil
}

// applyIssued 从 certmagic 存储里读回刚签发的 PEM 并落盘、热替换。
func (m *Manager) applyIssued(ctx context.Context, name string, data map[string]any) error {
	certKey, _ := data["certificate_path"].(string)
	keyKey, _ := data["private_key_path"].(string)
	if name == "" || certKey == "" || keyKey == "" {
		return errors.New("event without certificate paths")
	}
	certPEM, err := m.magic.Storage.Load(ctx, certKey)
	if err != nil {
		return fmt.Errorf("load stored certificate: %w", err)
	}
	keyPEM, err := m.magic.Storage.Load(ctx, keyKey)
	if err != nil {
		return fmt.Errorf("load stored key: %w", err)
	}
	return m.commit(normalizeName(name), certPEM, keyPEM)
}

// commit 校验证书与私钥配对，原子写盘，然后让监听器热替换。
func (m *Manager) commit(name string, certPEM, keyPEM []byte) error {
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("issued certificate and key do not match: %w", err)
	}
	certPath, keyPath := certPaths(name)
	if err := files.WriteAtomic(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", keyPath, err)
	}
	if err := files.WriteAtomic(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", certPath, err)
	}
	logging.ConsoleLog.Info().Msgf("acme: certificate for %s written to %s", name, certPath)
	if err := listener.ReloadCerts(); err != nil {
		return fmt.Errorf("reload certificates: %w", err)
	}
	return nil
}

// renewalWindowRatio 把「提前多少天续期」换成 certmagic 的剩余有效期比例。
// CA 常见有效期 90 天，30 天对应 1/3，与 certmagic 默认值一致。
func renewalWindowRatio(days int) float64 {
	if days <= 0 {
		return 0
	}
	ratio := time.Duration(days) * 24 * time.Hour
	value := float64(ratio) / float64(renewalCertLifetime)
	if value >= 1 {
		return 0.9
	}
	return value
}

// requiresEAB 报告该 CA 是否强制要求外部账户绑定。
func requiresEAB(ca config.ACMECA) bool {
	return strings.Contains(strings.ToLower(ca.Directory), "zerossl")
}

// DomainPlan 是配置里所有 ssl server_name 的 ACME 可用性划分。
type DomainPlan struct {
	Managed []string     // 会发起申请的域名（已小写、去重）
	Skipped []DomainSkip // 被跳过的名字与原因
}

// DomainSkip 记录一个不能走 ACME 的 server_name 及原因。
type DomainSkip struct {
	Name   string
	Reason string
}

// Plan 计算哪些 server_name 会走 ACME。纯函数、不写日志：
// acme setup 的摘要与 acme show 都用它，日志留到真正启动时再打。
func Plan(cfg *config.Fast_Https) DomainPlan {
	var plan DomainPlan
	if cfg == nil {
		return plan
	}
	seen := make(map[string]struct{}, len(cfg.Servers))
	for _, server := range cfg.Servers {
		if !strings.Contains(server.Listen, "ssl") {
			continue
		}
		name := normalizeName(server.ServerName)
		if ok, reason := acmeEligible(name); !ok {
			plan.Skipped = append(plan.Skipped, DomainSkip{Name: cmpOrName(server.ServerName, name), Reason: reason})
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		plan.Managed = append(plan.Managed, name)
	}
	return plan
}

// managedDomains 返回可以走 ACME 的 server_name，并把跳过的名字记进日志。
func managedDomains(cfg *config.Fast_Https) []string {
	plan := Plan(cfg)
	for _, skip := range plan.Skipped {
		logging.ConsoleLog.Warn().Msgf("acme: skipping server_name %q: %s", skip.Name, skip.Reason)
	}
	return plan.Managed
}

func cmpOrName(original, normalized string) string {
	if strings.TrimSpace(original) == "" {
		return normalized
	}
	return original
}

func acmeEligible(name string) (bool, string) {
	switch {
	case name == "":
		return false, "empty server_name"
	case strings.HasPrefix(name, "*."):
		return false, "wildcard names require dns-01, which is not supported"
	case net.ParseIP(name) != nil:
		return false, "IP addresses cannot be validated over http-01 or tls-alpn-01"
	case name == "localhost" || !strings.Contains(name, "."):
		return false, "not a public domain name"
	}
	return true, ""
}
