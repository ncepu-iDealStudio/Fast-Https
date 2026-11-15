package main

import (
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ruleBaseTime 是所有规则用例的零点，靠偏移量构造序列。
var ruleBaseTime = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func okObservation(at time.Time, pid int, startedAt string, total, fiveXX uint64, active int64) Observation {
	return Observation{
		At:   at,
		Kind: CollectOK,
		Raw:  json.RawMessage(`{"schema_version":"1"}`),
		Snapshot: Snapshot{
			SchemaVersion: SchemaVersion,
			Server:        ServerInfo{State: "running", PID: pid, StartedAt: startedAt},
			Requests:      RequestsInfo{Active: active, Total: total},
			Responses5xx:  fiveXX,
		},
	}
}

func failObservation(at time.Time, kind FailKind) Observation {
	return Observation{At: at, Kind: kind, Err: errors.New("collect failed")}
}

func newTestEngine(t *testing.T, mut func(*Config)) *Engine {
	t.Helper()
	return newTestEngineWithDial(t, mut, nil)
}

func newTestEngineWithDial(t *testing.T, mut func(*Config), dial dialer) *Engine {
	t.Helper()
	cfg := DefaultConfig()
	// 别读到仓库根目录里可能存在的 pid 文件。
	cfg.PidFile = filepath.Join(t.TempDir(), "fast-https.pid")
	if mut != nil {
		mut(&cfg)
	}
	return newEngine(cfg, dial)
}

func countType(events []Event, typ string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

// 必做规则 1：连续 3 次失败写「不可达」，之后一次成功写「恢复」。
func TestRuleUnreachableAndRecover(t *testing.T) {
	e := newTestEngine(t, nil)

	var unreachable, recovered int
	for i := 0; i < failThreshold; i++ {
		events := e.Observe(failObservation(ruleBaseTime.Add(time.Duration(i)*5*time.Second), FailUnreachable))
		unreachable += countType(events, EventUnreachable)
		if i < failThreshold-1 && countType(events, EventUnreachable) != 0 {
			t.Fatalf("run %d: unreachable fired before %d failures", i+1, failThreshold)
		}
	}
	if unreachable != 1 {
		t.Fatalf("unreachable events = %d, want 1", unreachable)
	}

	// 继续失败不重复写
	events := e.Observe(failObservation(ruleBaseTime.Add(20*time.Second), FailUnreachable))
	if countType(events, EventUnreachable) != 0 {
		t.Fatalf("unreachable repeated while still down: %v", events)
	}

	events = e.Observe(okObservation(ruleBaseTime.Add(25*time.Second), 7, "2026-11-08T09:00:00.000Z", 10, 0, 0))
	recovered = countType(events, EventRecovered)
	if recovered != 1 {
		t.Fatalf("recovered events = %d, want 1 (events=%v)", recovered, events)
	}

	// 恢复之后再失败，重新计数，不再写恢复
	events = e.Observe(okObservation(ruleBaseTime.Add(30*time.Second), 7, "2026-11-08T09:00:00.000Z", 20, 0, 0))
	if countType(events, EventRecovered) != 0 {
		t.Fatalf("recovered repeated: %v", events)
	}
}

// 必做规则 2：5xx 增量比例超过阈值且请求增量不少于 20。
func TestRule5xxRatio(t *testing.T) {
	t.Run("over threshold", func(t *testing.T) {
		e := newTestEngine(t, nil)
		e.Observe(okObservation(ruleBaseTime, 1, "s0", 1000, 0, 0))
		events := e.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 1, "s0", 1100, 20, 0))

		if n := countType(events, EventFiveXXRatio); n != 1 {
			t.Fatalf("5xx events = %d, want 1 (events=%v)", n, events)
		}
		ev := events[0]
		if ev.Severity != "warning" {
			t.Errorf("severity = %q, want warning", ev.Severity)
		}
		if ev.Summary == nil || ev.Summary.RequestsDelta != 100 || ev.Summary.Responses5xxDelta != 20 {
			t.Fatalf("summary = %+v, want delta 100/20", ev.Summary)
		}
	})

	t.Run("below min requests", func(t *testing.T) {
		e := newTestEngine(t, nil)
		e.Observe(okObservation(ruleBaseTime, 1, "s0", 1000, 0, 0))
		// 增量只有 10，比例再高也不该告警
		events := e.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 1, "s0", 1010, 8, 0))
		if n := countType(events, EventFiveXXRatio); n != 0 {
			t.Fatalf("5xx events = %d, want 0 when the request delta is too small (events=%v)", n, events)
		}
	})

	t.Run("healthy ratio", func(t *testing.T) {
		e := newTestEngine(t, nil)
		e.Observe(okObservation(ruleBaseTime, 1, "s0", 1000, 0, 0))
		events := e.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 1, "s0", 2000, 3, 0))
		if n := countType(events, EventFiveXXRatio); n != 0 {
			t.Fatalf("5xx events = %d, want 0 for a healthy ratio", n)
		}
	})
}

func TestRuleCooldown(t *testing.T) {
	e := newTestEngine(t, func(c *Config) { c.Cooldown = Duration(5 * time.Minute) })

	e.Observe(okObservation(ruleBaseTime, 1, "s0", 1000, 0, 0))
	if n := countType(e.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 1, "s0", 1100, 50, 0)), EventFiveXXRatio); n != 1 {
		t.Fatalf("first 5xx events = %d, want 1", n)
	}

	// 冷却期内继续超标：不重复写
	if n := countType(e.Observe(okObservation(ruleBaseTime.Add(10*time.Second), 1, "s0", 1200, 100, 0)), EventFiveXXRatio); n != 0 {
		t.Fatalf("5xx fired again inside the cooldown window")
	}

	// 冷却结束后再超标：可以再触发（先补一个样本把窗口建起来）
	e.Observe(okObservation(ruleBaseTime.Add(5*time.Minute), 1, "s0", 5000, 500, 0))
	events := e.Observe(okObservation(ruleBaseTime.Add(5*time.Minute+10*time.Second), 1, "s0", 5200, 620, 0))
	if n := countType(events, EventFiveXXRatio); n != 1 {
		t.Fatalf("5xx events after cooldown = %d, want 1 (events=%v)", n, events)
	}
}

// 进程重启后累计值从头开始：不算负增量，也不拿旧样本算比例。
func TestRuleCounterReset(t *testing.T) {
	e := newTestEngine(t, nil)
	e.Observe(okObservation(ruleBaseTime, 1, "s0", 1000, 0, 0))
	e.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 1, "s0", 1050, 1, 0))

	events := e.Observe(okObservation(ruleBaseTime.Add(10*time.Second), 2, "s1", 5, 0, 0))
	if n := countType(events, EventFiveXXRatio); n != 0 {
		t.Fatalf("counter reset produced a 5xx event: %v", events)
	}
	if n := countType(events, EventRestarted); n != 1 {
		t.Fatalf("restarted events = %d, want 1 (events=%v)", n, events)
	}

	// 重启后的增量从新基线算起
	events = e.Observe(okObservation(ruleBaseTime.Add(15*time.Second), 2, "s1", 105, 1, 0))
	if n := countType(events, EventFiveXXRatio); n != 0 {
		t.Fatalf("stale baseline survived the restart: %v", events)
	}
}

func TestRulePortProbe(t *testing.T) {
	listener := []ListenerInfo{{Port: "8080"}}

	t.Run("refused", func(t *testing.T) {
		var dialed string
		e := newTestEngineWithDial(t, func(c *Config) { c.PortProbe = true },
			func(addr string, _ time.Duration) (net.Conn, error) {
				dialed = addr
				return nil, errors.New("connection refused")
			})
		obs := okObservation(ruleBaseTime, 1, "s0", 10, 0, 0)
		obs.Snapshot.Listeners = listener
		events := e.Observe(obs)

		if n := countType(events, EventPortProbe); n != 1 {
			t.Fatalf("port events = %d, want 1 (events=%v)", n, events)
		}
		if dialed != "127.0.0.1:8080" {
			t.Errorf("dialed %q, want 127.0.0.1:8080", dialed)
		}
		if !strings.Contains(events[0].Message, "127.0.0.1:8080") {
			t.Errorf("message = %q, want it to name the address", events[0].Message)
		}
	})

	t.Run("up", func(t *testing.T) {
		e := newTestEngineWithDial(t, func(c *Config) { c.PortProbe = true },
			func(string, time.Duration) (net.Conn, error) { return fakeConn{}, nil })
		obs := okObservation(ruleBaseTime, 1, "s0", 10, 0, 0)
		obs.Snapshot.Listeners = listener
		if n := countType(e.Observe(obs), EventPortProbe); n != 0 {
			t.Fatalf("port events = %d, want 0 when the port answers", n)
		}
	})

	t.Run("wildcard maps to loopback", func(t *testing.T) {
		var dialed string
		e := newTestEngineWithDial(t, func(c *Config) { c.PortProbe = true },
			func(addr string, _ time.Duration) (net.Conn, error) {
				dialed = addr
				return fakeConn{}, nil
			})
		obs := okObservation(ruleBaseTime, 1, "s0", 10, 0, 0)
		obs.Snapshot.Listeners = []ListenerInfo{{Port: "0.0.0.0:443", SSL: true}}
		e.Observe(obs)
		if dialed != "127.0.0.1:443" {
			t.Errorf("dialed %q, want 127.0.0.1:443", dialed)
		}
	})

	t.Run("unparsable port is skipped", func(t *testing.T) {
		called := false
		e := newTestEngineWithDial(t, func(c *Config) { c.PortProbe = true },
			func(string, time.Duration) (net.Conn, error) {
				called = true
				return nil, errors.New("refused")
			})
		obs := okObservation(ruleBaseTime, 1, "s0", 10, 0, 0)
		obs.Snapshot.Listeners = []ListenerInfo{{Port: "not-a-port"}}
		if events := e.Observe(obs); len(events) != 0 {
			t.Fatalf("events = %v, want none", events)
		}
		if called {
			t.Error("unparsable port was dialed")
		}
	})

	t.Run("disabled by config", func(t *testing.T) {
		called := false
		e := newTestEngineWithDial(t, func(c *Config) { c.PortProbe = false },
			func(string, time.Duration) (net.Conn, error) {
				called = true
				return nil, errors.New("refused")
			})
		obs := okObservation(ruleBaseTime, 1, "s0", 10, 0, 0)
		obs.Snapshot.Listeners = listener
		if events := e.Observe(obs); len(events) != 0 {
			t.Fatalf("events = %v, want none with port_probe off", events)
		}
		if called {
			t.Error("port probe ran while disabled")
		}
	})
}

func TestRuleStuckHint(t *testing.T) {
	t.Run("active without progress", func(t *testing.T) {
		e := newTestEngine(t, func(c *Config) { c.StuckHint = true })
		var stuck int
		for i := 0; i <= 15; i++ {
			at := ruleBaseTime.Add(time.Duration(i) * 5 * time.Second)
			stuck += countType(e.Observe(okObservation(at, 1, "s0", 100, 0, 3)), EventStuck)
		}
		if stuck != 1 {
			t.Fatalf("stuck events = %d, want 1 (cooldown suppresses the rest)", stuck)
		}
	})

	t.Run("progress means no hint", func(t *testing.T) {
		e := newTestEngine(t, func(c *Config) { c.StuckHint = true })
		var stuck int
		for i := 0; i <= 15; i++ {
			at := ruleBaseTime.Add(time.Duration(i) * 5 * time.Second)
			stuck += countType(e.Observe(okObservation(at, 1, "s0", 100+uint64(i), 0, 3)), EventStuck)
		}
		if stuck != 0 {
			t.Fatalf("stuck events = %d, want 0 while requests keep completing", stuck)
		}
	})

	t.Run("idle means no hint", func(t *testing.T) {
		e := newTestEngine(t, func(c *Config) { c.StuckHint = true })
		var stuck int
		for i := 0; i <= 15; i++ {
			at := ruleBaseTime.Add(time.Duration(i) * 5 * time.Second)
			stuck += countType(e.Observe(okObservation(at, 1, "s0", 100, 0, 0)), EventStuck)
		}
		if stuck != 0 {
			t.Fatalf("stuck events = %d, want 0 when there is no active request", stuck)
		}
	})
}

// 事件要含时间、规则名、严重级别与快照摘要，且不含敏感信息。
func TestEventFields(t *testing.T) {
	e := newTestEngine(t, nil)

	var unreachable Event
	for i := 0; i < failThreshold; i++ {
		for _, ev := range e.Observe(failObservation(ruleBaseTime.Add(time.Duration(i)*5*time.Second), FailUnreachable)) {
			if ev.Type == EventUnreachable {
				unreachable = ev
			}
		}
	}
	if unreachable.Time != "2026-11-08T10:00:10.000Z" {
		t.Errorf("time = %q", unreachable.Time)
	}
	if unreachable.Rule != EventUnreachable || unreachable.Severity != "error" || unreachable.Message == "" {
		t.Fatalf("unreachable event = %+v", unreachable)
	}

	e2 := newTestEngine(t, nil)
	e2.Observe(okObservation(ruleBaseTime, 42, "2026-11-08T09:00:00.000Z", 1000, 0, 0))
	events := e2.Observe(okObservation(ruleBaseTime.Add(5*time.Second), 42, "2026-11-08T09:00:00.000Z", 1100, 20, 0))
	if len(events) != 1 {
		t.Fatalf("events = %v, want one 5xx event", events)
	}
	ev := events[0]
	if ev.Rule != EventFiveXXRatio || ev.Severity != "warning" {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Summary == nil || ev.Summary.PID != 42 || ev.Summary.State != "running" {
		t.Fatalf("summary = %+v, want the triggering snapshot", ev.Summary)
	}

	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	for _, banned := range []string{"blacklist", "ssl_certificate", "authorization", "cookie", "upstream_addr"} {
		if strings.Contains(strings.ToLower(string(raw)), banned) {
			t.Errorf("event contains forbidden word %q: %s", banned, raw)
		}
	}
}

// fakeConn 只用来表示「拨得通」，不参与任何读写。
type fakeConn struct{}

func (fakeConn) Read([]byte) (int, error)         { return 0, errFakeConn }
func (fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (fakeConn) Close() error                     { return nil }
func (fakeConn) LocalAddr() net.Addr              { return fakeAddr{} }
func (fakeConn) RemoteAddr() net.Addr             { return fakeAddr{} }
func (fakeConn) SetDeadline(time.Time) error      { return nil }
func (fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeConn) SetWriteDeadline(time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "tcp" }
func (fakeAddr) String() string  { return "127.0.0.1:0" }

var errFakeConn = errors.New("fake connection")
