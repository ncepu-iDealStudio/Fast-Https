package safe

import (
	"net"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/runtime/request"
)

// resetOverlay 清空控制面 overlay 并在测试结束后再次清空，
// 避免用例间通过全局 overlay 串状态。
func resetOverlay(t *testing.T) {
	t.Helper()
	clear := func() {
		runtimeOverlay.mu.Lock()
		runtimeOverlay.added = map[string]bool{}
		runtimeOverlay.removed = map[string]bool{}
		runtimeOverlay.rateSet = false
		runtimeOverlay.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// 控制面加黑名单：立即生效，且 reload（blacklistInit 重建）后以
// overlay 存活
func TestControlAddBlacklistSurvivesReload(t *testing.T) {
	resetOverlay(t)
	blacklistInit()

	if err := AddBlacklist("203.0.113.9"); err != nil {
		t.Fatal(err)
	}
	if !g_list.Contains("203.0.113.9") {
		t.Fatal("runtime add should take effect immediately")
	}

	blacklistInit() // 模拟 reload 重建
	if !g_list.Contains("203.0.113.9") {
		t.Error("runtime addition should survive reload via overlay")
	}

	src := ""
	for _, e := range ListBlacklist() {
		if e.Raw == "203.0.113.9" {
			src = e.Source
		}
	}
	if src != BlacklistSourceRuntime {
		t.Errorf("source = %q, want %q", src, BlacklistSourceRuntime)
	}
}

// 控制面移除配置录入：立即生效且 reload 后仍被移除
func TestControlRemoveBlacklistSurvivesReload(t *testing.T) {
	resetOverlay(t)
	prev := config.Current().BlackList
	config.Current().BlackList = []string{"198.51.100.7"}
	t.Cleanup(func() { config.Current().BlackList = prev })
	blacklistInit()

	if !g_list.Contains("198.51.100.7") {
		t.Fatal("config entry should be loaded")
	}
	if err := RemoveBlacklist("198.51.100.7"); err != nil {
		t.Fatal(err)
	}
	if g_list.Contains("198.51.100.7") {
		t.Fatal("remove should take effect immediately")
	}

	blacklistInit() // 模拟 reload 重建
	if g_list.Contains("198.51.100.7") {
		t.Error("removed config entry should stay removed after reload")
	}
}

// 重复录入与移除不存在的录入都要报错
func TestControlBlacklistErrors(t *testing.T) {
	resetOverlay(t)
	blacklistInit()

	if err := AddBlacklist("203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	if err := AddBlacklist("203.0.113.10"); err == nil {
		t.Error("duplicate entry should fail")
	}
	if err := RemoveBlacklist("203.0.113.99"); err == nil {
		t.Error("removing an absent entry should fail")
	}
	if err := AddBlacklist("not-an-ip"); err == nil {
		t.Error("invalid entry should fail")
	}
}

// 控制面调参：立即改变限流行为，reload 后以 overlay 存活
func TestControlSetGlobalRateLimit(t *testing.T) {
	resetOverlay(t)
	limitInit()

	if err := SetGlobalRateLimit(1, 1); err != nil {
		t.Fatal(err)
	}
	st := GetGlobalRateLimit()
	if st.Rate != 1 || st.Burst != 1 {
		t.Fatalf("state = %+v, want rate=1 burst=1", st)
	}
	if !g_ips.allow("9.9.9.9") {
		t.Fatal("first request should pass")
	}
	if g_ips.allow("9.9.9.9") {
		t.Error("burst exhausted should reject")
	}

	limitInit() // 模拟 reload 重建
	t.Logf("DBG state after limitInit: %+v buckets=%d", GetGlobalRateLimit(), GetGlobalRateLimit().Buckets)
	t.Logf("DBG allow fresh-ip: %v", g_ips.allow("7.7.7.7"))
	a9 := g_ips.allow("9.9.9.9")
	t.Logf("DBG allow 9.9.9.9 #1: %v buckets=%d", a9, GetGlobalRateLimit().Buckets)
	if !a9 {
		t.Error("overlay should restore the control-plane rate after reload")
	}
	a9 = g_ips.allow("9.9.9.9")
	t.Logf("DBG allow 9.9.9.9 #2: %v buckets=%d", a9, GetGlobalRateLimit().Buckets)
	st = GetGlobalRateLimit()
	// 两个 IP 的 allow 各自创建一个桶：7.7.7.7 + 9.9.9.9 = 2
	if st.Rate != 1 || st.Burst != 1 || st.Buckets != 2 {
		t.Errorf("after reload state = %+v", st)
	}
}

// 非法参数必须报错
func TestControlSetGlobalRateLimitRejectsInvalid(t *testing.T) {
	resetOverlay(t)
	limitInit()
	if err := SetGlobalRateLimit(0, 1); err == nil {
		t.Error("rate=0 should fail")
	}
	if err := SetGlobalRateLimit(-5, 1); err == nil {
		t.Error("negative rate should fail")
	}
	if err := SetGlobalRateLimit(1, -1); err == nil {
		t.Error("negative burst should fail")
	}
}

// 拒绝连接的 nop 实现：只提供地址
type controlNopConn struct {
	remote net.Addr
}

func (c *controlNopConn) Read(b []byte) (int, error)       { return 0, net.ErrClosed }
func (c *controlNopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *controlNopConn) Close() error                     { return nil }
func (c *controlNopConn) LocalAddr() net.Addr              { return c.remote }
func (c *controlNopConn) RemoteAddr() net.Addr             { return c.remote }
func (c *controlNopConn) SetDeadline(time.Time) error      { return nil }
func (c *controlNopConn) SetReadDeadline(time.Time) error  { return nil }
func (c *controlNopConn) SetWriteDeadline(time.Time) error { return nil }

// newControlEvent 构造测试事件：拒绝路径写 403 时吞掉写出。
// 对齐生产不变量：Bucket 在 parseRequest 之后调用，RR.Req 必非 nil。
func newControlEvent(ip1, ip2, ip3, ip4 byte) *core.Event {
	ev := &core.Event{Conn: &controlNopConn{
		remote: &net.TCPAddr{IP: net.IPv4(ip1, ip2, ip3, ip4), Port: 1000},
	}}
	ev.RR.Req = request.RequestInit(false)
	ev.EventWrite = func(*core.Event, []byte) error { return nil }
	return ev
}

// Snapshot 反映黑名单与拒绝计数
func TestSnapshotReflectsState(t *testing.T) {
	resetOverlay(t)
	blacklistInit()
	limitInit()

	if err := SetGlobalRateLimit(1, 1); err != nil {
		t.Fatal(err)
	}
	if err := AddBlacklist("203.0.113.77"); err != nil {
		t.Fatal(err)
	}

	ev := newControlEvent(203, 0, 113, 80)
	before := Snapshot()
	if !Bucket(ev) {
		t.Fatal("first request at rate=1 should pass")
	}
	if Bucket(ev) {
		t.Fatal("second request should be rejected")
	}
	after := Snapshot()

	if after.TotalChecked-before.TotalChecked != 2 {
		t.Errorf("total checked delta = %d, want 2", after.TotalChecked-before.TotalChecked)
	}
	if after.RateLimitRejected-before.RateLimitRejected != 1 {
		t.Errorf("rate limit rejected delta = %d, want 1", after.RateLimitRejected-before.RateLimitRejected)
	}
	// 两次 Bucket 不改变黑名单
	if after.BlacklistSize != before.BlacklistSize {
		t.Errorf("blacklist size changed: %d -> %d", before.BlacklistSize, after.BlacklistSize)
	}
	if after.GlobalRate != 1 || after.GlobalBurst != 1 {
		t.Errorf("global rate/burst = %d/%d, want 1/1", after.GlobalRate, after.GlobalBurst)
	}

	// 黑名单命中的计数
	banEv := newControlEvent(203, 0, 113, 77)
	beforeBanned := Snapshot().BlacklistRejected
	if !IsInBlacklist(banEv) {
		t.Fatal("blacklisted ip should be rejected")
	}
	if Snapshot().BlacklistRejected-beforeBanned != 1 {
		t.Error("blacklist rejected counter should increment")
	}
}
