package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// 规则口径的固定值。阈值与冷却由配置决定，这几个是口径本身，不做成配置项。
const (
	// failThreshold 是连续多少次采集失败后写「不可达」。
	failThreshold = 3
	// ratioWindow 是「5xx 升高」的统计窗口。
	ratioWindow = time.Minute
	// stuckAfter 是「疑似卡死」要求的静默时长。
	stuckAfter = time.Minute
	// portProbeTimeout 是单次业务端口探测的超时。
	portProbeTimeout = 300 * time.Millisecond
)

// dialer 让端口探测在测试里可替换，生产用真实 TCP 连接。
type dialer func(addr string, timeout time.Duration) (net.Conn, error)

func defaultDial(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// counterSample 是一次采集里参与窗口计算的累计值。
type counterSample struct {
	at       time.Time
	requests uint64
	fiveXX   uint64
}

// Engine 是全部确定性规则的集合：不可达 / 恢复、进程重启、5xx 比例、
// 业务端口探测、疑似卡死。全是确定性判断，不用模型、不做统计推断。
//
// 它只读 Observation 与自身状态：不打日志、不写文件、不发告警、不改 Server。
type Engine struct {
	cooldown    time.Duration
	ratio       float64
	minRequests uint64
	portProbe   bool
	stuckHint   bool
	pidFile     string
	dial        dialer

	failStreak  int
	unreachable bool
	lastKind    FailKind

	haveBase  bool
	pid       int
	startedAt string

	samples   []counterSample
	lastFired map[string]time.Time

	portState map[string]bool

	stuckSince     time.Time
	stuckLastTotal uint64
	haveStuckBase  bool
}

// newEngine 按配置生成规则引擎。dial 为 nil 时用真实的 TCP 连接探测。
func newEngine(cfg Config, dial dialer) *Engine {
	if dial == nil {
		dial = defaultDial
	}
	ratio := cfg.FiveXXRatio
	if ratio <= 0 {
		ratio = DefaultFiveXXRatio
	}
	minReq := cfg.FiveXXMinRequests
	if minReq <= 0 {
		minReq = DefaultFiveXXMinRequests
	}
	return &Engine{
		cooldown:    cfg.Cooldown.D(),
		ratio:       ratio,
		minRequests: uint64(minReq),
		portProbe:   cfg.PortProbe,
		stuckHint:   cfg.StuckHint,
		pidFile:     cfg.PidFile,
		dial:        dial,
		lastFired:   map[string]time.Time{},
		portState:   map[string]bool{},
	}
}

// Observe 把一次采集结果翻译成事件。时间取 obs.At，测试可以直接给假时间。
func (e *Engine) Observe(obs Observation) []Event {
	if obs.Kind != CollectOK {
		return e.onFailure(obs)
	}
	return e.onSuccess(obs)
}

// FailureText 把失败类别与 PID 文件的判断合成一句话，供状态文件与事件共用。
func (e *Engine) FailureText(obs Observation) string {
	text := fmt.Sprintf("%s: %v", obs.Kind, obs.Err)
	if obs.Kind != FailUnreachable {
		return text
	}
	return text + "; " + e.serverVerdict()
}

func (e *Engine) onFailure(obs Observation) []Event {
	e.failStreak++
	var events []Event
	// 坏法变了就先记一条：非 200、坏 JSON、版本不认识都不该等到第三次才暴露。
	if obs.Kind != e.lastKind {
		if ev, ok := e.fire(obs, EventCollectError, "warning", e.FailureText(obs), nil, true); ok {
			events = append(events, ev)
		}
	}
	e.lastKind = obs.Kind
	if e.failStreak >= failThreshold && !e.unreachable {
		e.unreachable = true
		msg := fmt.Sprintf("%d consecutive collection failures: %s", e.failStreak, e.FailureText(obs))
		if ev, ok := e.fire(obs, EventUnreachable, "error", msg, nil, false); ok {
			events = append(events, ev)
		}
	}
	// 采集断了，窗口跟着作废：恢复后不能拿断档前的样本算增量。
	e.samples = nil
	e.resetStuck()
	return events
}

func (e *Engine) onSuccess(obs Observation) []Event {
	var events []Event
	if e.unreachable {
		e.unreachable = false
		if ev, ok := e.fire(obs, EventRecovered, "info", "snapshot is readable again", summaryOf(obs.Snapshot), false); ok {
			events = append(events, ev)
		}
	}

	// 进程重启：PID 或 started_at 变化。累计值从头开始，窗口一起清空。
	restarted := e.haveBase && (obs.Snapshot.Server.PID != e.pid || obs.Snapshot.Server.StartedAt != e.startedAt)
	if restarted {
		msg := fmt.Sprintf("server restarted: pid %d -> %d, started_at %q -> %q",
			e.pid, obs.Snapshot.Server.PID, e.startedAt, obs.Snapshot.Server.StartedAt)
		if ev, ok := e.fire(obs, EventRestarted, "warning", msg, summaryOf(obs.Snapshot), false); ok {
			events = append(events, ev)
		}
		e.samples = nil
		e.resetStuck()
	}

	e.haveBase = true
	e.pid = obs.Snapshot.Server.PID
	e.startedAt = obs.Snapshot.Server.StartedAt
	e.failStreak = 0
	e.lastKind = CollectOK

	if ev, ok := e.ratioRule(obs); ok {
		events = append(events, ev)
	}
	events = append(events, e.portRule(obs)...)
	if ev, ok := e.stuckRule(obs); ok {
		events = append(events, ev)
	}
	return events
}

// ratioRule 是「5xx 升高」：最近一分钟的 5xx 增量除以请求增量超过阈值，
// 且请求增量不少于下限（避免除以一个很小的数）。
func (e *Engine) ratioRule(obs Observation) (Event, bool) {
	cur := counterSample{
		at:       obs.At,
		requests: obs.Snapshot.Requests.Total,
		fiveXX:   obs.Snapshot.Responses5xx,
	}
	if n := len(e.samples); n > 0 {
		last := e.samples[n-1]
		// 计数回落说明进程重启过：不算负增量，直接换窗口。
		if cur.requests < last.requests || cur.fiveXX < last.fiveXX {
			e.samples = nil
		}
	}
	e.samples = append(e.samples, cur)
	e.pruneSamples(obs.At)
	if len(e.samples) < 2 {
		return Event{}, false
	}

	base := e.samples[0]
	if cur.requests < base.requests || cur.fiveXX < base.fiveXX {
		return Event{}, false
	}
	deltaReq := cur.requests - base.requests
	if deltaReq == 0 || deltaReq < e.minRequests {
		return Event{}, false
	}
	delta5xx := cur.fiveXX - base.fiveXX
	got := float64(delta5xx) / float64(deltaReq)
	if got < e.ratio {
		return Event{}, false
	}

	summary := summaryOf(obs.Snapshot)
	summary.RequestsDelta = deltaReq
	summary.Responses5xxDelta = delta5xx
	msg := fmt.Sprintf("5xx ratio %.2f%% over the last %s exceeds %.2f%% (%d of %d requests)",
		got*100, ratioWindow, e.ratio*100, delta5xx, deltaReq)
	return e.fire(obs, EventFiveXXRatio, "warning", msg, summary, true)
}

// pruneSamples 只留窗口内的样本；窗口外的老样本不再参与增量。
func (e *Engine) pruneSamples(now time.Time) {
	cutoff := now.Add(-ratioWindow)
	keep := e.samples[:0]
	for _, s := range e.samples {
		if !s.at.Before(cutoff) {
			keep = append(keep, s)
		}
	}
	e.samples = keep
}

// portRule 是「业务端口不可达」：/runtime 正常但监听端口连不上。
// 只在端口从可用变不可用时写一条，端口表里没了的地址顺手清掉状态。
func (e *Engine) portRule(obs Observation) []Event {
	if !e.portProbe || len(obs.Snapshot.Listeners) == 0 {
		return nil
	}
	targets := make([]string, 0, len(obs.Snapshot.Listeners))
	seen := make(map[string]bool, len(obs.Snapshot.Listeners))
	for _, lis := range obs.Snapshot.Listeners {
		addr, ok := probeAddr(lis.Port)
		if !ok || seen[addr] {
			continue
		}
		seen[addr] = true
		targets = append(targets, addr)
	}
	if len(targets) == 0 {
		return nil
	}

	// 并发探测：串行的话端口一多就会把采集周期吃满。
	type probeResult struct {
		addr string
		up   bool
	}
	results := make(chan probeResult, len(targets))
	for _, addr := range targets {
		go func(addr string) {
			conn, err := e.dial(addr, portProbeTimeout)
			if conn != nil {
				_ = conn.Close()
			}
			results <- probeResult{addr: addr, up: err == nil}
		}(addr)
	}

	var events []Event
	for i := 0; i < len(targets); i++ {
		r := <-results
		prev, known := e.portState[r.addr]
		e.portState[r.addr] = r.up
		if r.up || (known && !prev) {
			// 通的，或者上次就已经判过不通：不重复写。
			continue
		}
		msg := fmt.Sprintf("listener %s is not reachable while /runtime is up", r.addr)
		if ev, ok := e.fire(obs, EventPortProbe, "warning", msg, summaryOf(obs.Snapshot), true); ok {
			events = append(events, ev)
		}
	}
	for addr := range e.portState {
		if !seen[addr] {
			delete(e.portState, addr)
		}
	}
	return events
}

// stuckRule 是「疑似卡死」提示：活动请求长时间不降，且累计请求不再增长。
// 只提示，不下结论，也不做任何自动处理。
func (e *Engine) stuckRule(obs Observation) (Event, bool) {
	if !e.stuckHint {
		return Event{}, false
	}
	active := obs.Snapshot.Requests.Active
	total := obs.Snapshot.Requests.Total
	if active <= 0 {
		e.resetStuck()
		return Event{}, false
	}
	if !e.haveStuckBase || total != e.stuckLastTotal {
		e.haveStuckBase = true
		e.stuckLastTotal = total
		e.stuckSince = obs.At
		return Event{}, false
	}
	if obs.At.Sub(e.stuckSince) < stuckAfter {
		return Event{}, false
	}
	msg := fmt.Sprintf("requests.active=%d has not dropped and requests.total has not grown for %s (hint only)",
		active, stuckAfter)
	return e.fire(obs, EventStuck, "info", msg, summaryOf(obs.Snapshot), true)
}

func (e *Engine) resetStuck() {
	e.stuckSince = time.Time{}
	e.stuckLastTotal = 0
	e.haveStuckBase = false
}

// fire 组装一条事件。useCooldown 为真时，同一条规则在冷却期内不重复写。
func (e *Engine) fire(obs Observation, rule, severity, message string, summary *EventSummary, useCooldown bool) (Event, bool) {
	if useCooldown && e.cooldown > 0 {
		if last, ok := e.lastFired[rule]; ok && obs.At.Sub(last) < e.cooldown {
			return Event{}, false
		}
	}
	e.lastFired[rule] = obs.At
	return Event{
		Time:     obs.At.Format(TimeFormat),
		Type:     rule,
		Rule:     rule,
		Severity: severity,
		Message:  message,
		Summary:  summary,
	}, true
}

// serverVerdict 在 /runtime 连不上时读 PID 文件，区分从未启动、已崩溃、启动中。
func (e *Engine) serverVerdict() string {
	pid, state, err := ReadPidFile(e.pidFile)
	if err != nil {
		return "pid file: " + err.Error()
	}
	switch state {
	case PidAbsent:
		return "server not started (no pid file)"
	case PidStale:
		return fmt.Sprintf("server crashed (pid %d is gone)", pid)
	case PidAlive:
		return fmt.Sprintf("server is starting (pid %d alive, /runtime not answering)", pid)
	}
	return "server state unknown"
}

// probeAddr 把 /runtime 报的监听端口换成本机可拨的地址。
// 只拨回环：目的是「业务端口还能不能连」，不是远程巡检。
func probeAddr(port string) (string, bool) {
	p := strings.TrimSpace(port)
	if p == "" {
		return "", false
	}
	host := ""
	if i := strings.LastIndex(p, ":"); i >= 0 {
		host = strings.Trim(p[:i], "[]")
		p = p[i+1:]
	}
	if _, err := strconv.Atoi(p); err != nil {
		return "", false
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, p), true
}

func summaryOf(snap Snapshot) *EventSummary {
	return &EventSummary{
		State:         snap.Server.State,
		PID:           snap.Server.PID,
		StartedAt:     snap.Server.StartedAt,
		RequestsTotal: snap.Requests.Total,
		Responses5xx:  snap.Responses5xx,
	}
}
