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
	cfg *config.AcmeConfig
	// names 是这次运行托管的域名集合。保留它是为了 reload 时比较：
	// 域名变了要重建管理器，配置没变就不该打断续期调度。
	names []string
	magic *certmagic.Config
	cache *certmagic.Cache
	http  *http01Solver
	alpn  *tlsalpn01Solver

	cancel context.CancelFunc
	once   sync.Once
	// stopped 记录这个实例是否已经停掉。给测试与排错用：
	// reload 重建管理器时靠它断言旧实例确实被停了。
	stopped atomic.Bool
}

var (
	activeManager atomic.Pointer[Manager]
	startMu       sync.Mutex
)

// startBackground 是 m.run 的间接引用，测试把它换成空实现，
// 避免 Reconcile 真的向 CA 发起申请。
var startBackground = func(m *Manager) { m.run() }

// ReconcileHooks 按当前配置快照挂上或撤销数据面钩子。幂等，可反复调用。
//
// 调用位置有约束：必须在监听器重载**之前**跑。listenSsl 在构造 tls.Config
// 时读 listener.TLSALPNEnabled()，provider 晚一步注册，新建出来的监听器
// 就不会通告 acme-tls/1，tls-alpn-01 必然失败。
func ReconcileHooks() {
	cfg := config.Current().Acme
	if !cfg.Active() {
		core.SetEarlyRequestHandler(nil)
		listener.RegisterTLSALPNProvider(nil)
		return
	}
	core.SetEarlyRequestHandler(handleEarlyRequest)
	// 只有配置里启用了 tls-alpn-01 才通告 acme-tls/1 并注入挑战证书来源。
	if cfg.UsesChallenge(config.ACMEChallengeTLSALPN01) {
		listener.RegisterTLSALPNProvider(tlsALPNChallengeCert)
		return
	}
	listener.RegisterTLSALPNProvider(nil)
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

// Reconcile 按当前配置快照对齐后台任务：运行期没有差异时什么都不做，
// 有差异时按新配置重新起一个管理器。启动与每次 reload 都走这里。
//
// 只在有差异时重建是有意的：无条件重建会取消 certmagic 的退避队列与
// 进行中的订单，既丢进度也可能撞上 CA 的速率限制。
func Reconcile() {
	snapshot := config.Current()
	cfg := snapshot.Acme
	// 先算域名，但不打日志：没有差异时每次 reload 都刷一遍跳过原因太吵。
	plan := Plan(snapshot)

	startMu.Lock()
	defer startMu.Unlock()

	prev := activeManager.Load()

	if !cfg.Active() {
		if prev == nil {
			return
		}
		activeManager.Store(nil)
		prev.stop()
		logging.ConsoleLog.Info().Msg("acme: manager stopped (reason: acme disabled)")
		return
	}

	reason := ""
	if prev != nil {
		switch diff := acmeDiff(prev.cfg, cfg); {
		case diff != "":
			reason = diff
		case !sameNames(prev.names, plan.Managed):
			reason = "domains"
		default:
			return
		}
	} else {
		reason = "enabled"
	}

	if len(plan.Managed) == 0 {
		if prev != nil {
			activeManager.Store(nil)
			prev.stop()
			logging.ConsoleLog.Info().Msg("acme: manager stopped (reason: no domain can use ACME)")
			return
		}
		logSkippedDomains(plan)
		logging.ConsoleLog.Warn().Msg("acme: no server_name can use ACME, keeping self-signed certificates")
		return
	}

	m, err := newManager(cfg, plan.Managed)
	if err != nil {
		// 构造失败不动旧的：宁可继续用旧配置续期，也不要因为一次写错就停掉证书链路。
		logging.ConsoleLog.Error().Msgf("acme: kept previous manager, new config is unusable: %v", err)
		return
	}

	// 先换指针再停旧实例：请求路径要么看到旧管理器（此时还没停），要么看到新的。
	activeManager.Store(m)
	if prev != nil {
		prev.stop()
	}
	logSkippedDomains(plan)
	startBackground(m)

	switch {
	case prev == nil:
		logging.ConsoleLog.Info().Msgf("acme: background issuance started for %s", strings.Join(plan.Managed, ", "))
	default:
		logging.ConsoleLog.Info().Msgf("acme: manager restarted, background issuance started for %s (reason: %s)",
			strings.Join(plan.Managed, ", "), reason)
	}
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
	m.stopped.Store(true)
}

// acmeDiff 报告两份 acme 配置在运行期是否有差异，返回差异字段名；
// 空串表示运行期等价，不需要重建管理器。
//
// fallback_self_signed 不参与比较：它只在 CLI 摘要与配置校验里用，运行期不读。
// ssl_certificate 路径也不参与：certPaths() 每次都读当前快照。
func acmeDiff(old, next *config.AcmeConfig) string {
	if old.Active() != next.Active() {
		return "enabled"
	}
	if !old.Active() {
		return ""
	}
	switch {
	case old.Email != next.Email:
		return "email"
	case old.Staging != next.Staging:
		return "staging"
	case old.RenewBeforeDays != next.RenewBeforeDays:
		return "renew_before_days"
	case old.Storage != next.Storage:
		return "storage"
	case !sameChallenges(old, next):
		return "challenge"
	case !sameCAs(old.CAs, next.CAs):
		return "cas"
	}
	return ""
}

// sameChallenges 按「实际启用的挑战集合」比较：
// auto 等价于 http-01 + tls-alpn-01，写法的顺序与展开形式不影响语义。
func sameChallenges(old, next *config.AcmeConfig) bool {
	a := effectiveChallenges(old)
	b := effectiveChallenges(next)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// effectiveChallenges 把 auto 展开成实际会用的挑战集合，顺序固定。
func effectiveChallenges(cfg *config.AcmeConfig) []string {
	var out []string
	for _, name := range []string{config.ACMEChallengeHTTP01, config.ACMEChallengeTLSALPN01} {
		if cfg.UsesChallenge(name) {
			out = append(out, name)
		}
	}
	return out
}

// sameCAs 逐项比较降级链：顺序有意义（IssuerPolicy 用第一条可用 CA）。
func sameCAs(a, b []config.ACMECA) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameNames 按集合比较域名：配置里 server 块的排列顺序变化不该重启管理器。
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]struct{}, len(a))
	for _, name := range a {
		seen[name] = struct{}{}
	}
	for _, name := range b {
		if _, ok := seen[name]; !ok {
			return false
		}
	}
	return true
}

func newManager(cfg *config.AcmeConfig, names []string) (*Manager, error) {
	if err := os.MkdirAll(cfg.Storage, 0o755); err != nil {
		return nil, fmt.Errorf("create acme storage %s: %w", cfg.Storage, err)
	}

	m := &Manager{cfg: cfg, names: append([]string(nil), names...)}
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
func (m *Manager) run() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	names := m.names

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

// logSkippedDomains 把不能走 ACME 的 server_name 与原因记进日志。
// 只在真正（重）启动管理器时调用：没有差异的 reload 不该刷一串重复告警。
func logSkippedDomains(plan DomainPlan) {
	for _, skip := range plan.Skipped {
		logging.ConsoleLog.Warn().Msgf("acme: skipping server_name %q: %s", skip.Name, skip.Reason)
	}
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
