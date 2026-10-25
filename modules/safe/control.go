// control.go 是 safe 模块的控制面接口：供未来的 agent/管理端调用，
// 对数据面做运行时变更（加删黑名单、调整限流）与状态查询。
//
// 语义约定：
//   - 运行时变更立即生效，并以 overlay 形式在 reload 后保留
//     （blacklistInit/limitInit 重建配置底座后会重新应用 overlay）；
//     控制面若要持久化，由 agent 侧自行存储并回放。
//   - 所有变更写安全审计日志（reason=control），供追溯。
package safe

import (
	"fmt"
	"sync"

	"fast-https/infra/logging"
)

// 黑名单录入来源。
const (
	BlacklistSourceConfig  = "config"
	BlacklistSourceRuntime = "runtime"
)

// runtimeOverlay 记录控制面在运行时做的变更，reload 后由
// applyBlacklistOverlay / applyRateOverlay 重新应用。
var runtimeOverlay = struct {
	mu      sync.Mutex
	added   map[string]bool // 控制面加入的录入
	removed map[string]bool // 控制面移除的配置录入
	rateSet bool
	rate    int
	burst   int
}{
	added:   make(map[string]bool),
	removed: make(map[string]bool),
}

// AddBlacklist 把 IP/段加入黑名单（控制面入口）。
// 立即生效，reload 后保留。
func AddBlacklist(entry string) error {
	if err := g_list.Add(BlacklistSourceRuntime, entry); err != nil {
		return err
	}

	runtimeOverlay.mu.Lock()
	runtimeOverlay.added[entry] = true
	delete(runtimeOverlay.removed, entry)
	runtimeOverlay.mu.Unlock()

	logControl("add_blacklist", entry)
	return nil
}

// RemoveBlacklist 把 IP/段移出黑名单（控制面入口）。
// 立即生效，reload 后保留。
func RemoveBlacklist(entry string) error {
	if err := g_list.Remove(entry); err != nil {
		return err
	}

	runtimeOverlay.mu.Lock()
	delete(runtimeOverlay.added, entry)
	runtimeOverlay.removed[entry] = true
	runtimeOverlay.mu.Unlock()

	logControl("remove_blacklist", entry)
	return nil
}

// ListBlacklist 返回当前黑名单的全部录入（含来源）。
func ListBlacklist() []BlacklistEntry {
	return g_list.Entries()
}

// SetGlobalRateLimit 调整全局 per-IP 限流参数（控制面入口）。
// rate 为每秒请求数，burst 为桶容量；既有 IP 桶清空后按新参数重建。
// 立即生效，reload 后保留。
func SetGlobalRateLimit(rate, burst int) error {
	if rate <= 0 {
		return fmt.Errorf("rate 必须大于 0，得到 %d", rate)
	}
	if burst < 0 {
		return fmt.Errorf("burst 不能为负，得到 %d", burst)
	}
	g_ips.setRate(rate, burst)

	runtimeOverlay.mu.Lock()
	runtimeOverlay.rateSet = true
	runtimeOverlay.rate = rate
	runtimeOverlay.burst = burst
	runtimeOverlay.mu.Unlock()

	logControl("set_rate_limit", fmt.Sprintf("rate=%d burst=%d", rate, burst))
	return nil
}

// GetGlobalRateLimit 返回当前全局限流参数与活跃 IP 桶数。
func GetGlobalRateLimit() RateLimitState {
	return g_ips.state()
}

// applyBlacklistOverlay 在 blacklistInit 重建配置底座后重新应用
// 控制面的运行时变更：先删掉被移除的配置录入，再加入运行时录入。
func applyBlacklistOverlay() {
	runtimeOverlay.mu.Lock()
	removed := make([]string, 0, len(runtimeOverlay.removed))
	for entry := range runtimeOverlay.removed {
		removed = append(removed, entry)
	}
	added := make([]string, 0, len(runtimeOverlay.added))
	for entry := range runtimeOverlay.added {
		added = append(added, entry)
	}
	runtimeOverlay.mu.Unlock()

	for _, entry := range removed {
		_ = g_list.Remove(entry)
	}
	for _, entry := range added {
		_ = g_list.Add(BlacklistSourceRuntime, entry)
	}
}

// applyRateOverlay 在 limitInit 重建后重新应用控制面的限流参数。
func applyRateOverlay() {
	println("APPLYRATE enter")
	runtimeOverlay.mu.Lock()
	set, rate, burst := runtimeOverlay.rateSet, runtimeOverlay.rate, runtimeOverlay.burst
	runtimeOverlay.mu.Unlock()

	if set {
		g_ips.setRate(rate, burst)
	}
}

// logControl 写一条控制面审计日志。
func logControl(action, detail string) {
	logging.Safe().Info().
		Str(logging.FieldReason, "control").
		Str("action", action).
		Str("detail", detail).
		Msg("control plane changed data plane state")
}
