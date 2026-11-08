package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// failThreshold 是连续多少次采集失败后写一条「不可达」。
// 一次抖动不算：单次失败只记进状态文件，不进事件。
const failThreshold = 3

// Runner 是 Monitor 的判定中枢：采一次、判一次、落一次盘。
// 它只读 /runtime 与 PID 文件，不写 Server，也不进入请求路径。
type Runner struct {
	cfg    Config
	col    *Collector
	sink   *Sink
	clk    Clock
	logger *log.Logger

	failStreak  int
	alerted     bool
	lastKind    FailKind
	haveBase    bool
	pid         int
	startedAt   string
	lastEventAt string
}

// NewRunner 按配置组装采集器与落盘器。clk 与 logger 传 nil 时用系统时钟与标准错误。
func NewRunner(cfg Config, clk Clock, logger *log.Logger) *Runner {
	if clk == nil {
		clk = SystemClock()
	}
	if logger == nil {
		logger = log.New(os.Stderr, "monitor: ", log.LstdFlags)
	}
	return &Runner{
		cfg:    cfg,
		clk:    clk,
		logger: logger,
		col:    &Collector{URL: cfg.SnapshotURL, Timeout: cfg.Timeout.D()},
		sink:   &Sink{Dir: cfg.Dir, EventMaxBytes: cfg.EventMaxBytes, EventKeep: cfg.EventKeep},
	}
}

// RunOnce 采一次、判一次、落一次盘。返回采集结果与本轮产生的事件。
func (r *Runner) RunOnce(ctx context.Context) (Observation, []Event) {
	obs := r.col.Collect(ctx, r.clk.Now())
	events := r.decide(obs)
	for i := range events {
		events[i].Time = obs.At.Format(TimeFormat)
	}

	if obs.Kind == CollectOK {
		if err := r.sink.AppendSnapshot(obs.Raw); err != nil {
			r.logger.Printf("append snapshot: %v", err)
		}
	}
	for _, ev := range events {
		if err := r.sink.AppendEvent(ev); err != nil {
			r.logger.Printf("append event: %v", err)
			continue
		}
		r.lastEventAt = ev.Time
	}
	if err := r.sink.WriteStatus(r.statusOf(obs)); err != nil {
		r.logger.Printf("write status: %v", err)
	}
	return obs, events
}

// Run 先立即采一次，之后按周期采集，直到 ctx 结束。
// 单元测试用 RunOnce 驱动，不会真的跑这个循环。
func (r *Runner) Run(ctx context.Context) error {
	r.RunOnce(ctx)
	ticker := time.NewTicker(r.cfg.Interval.D())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

// decide 把一次采集结果翻译成事件，并维护跨轮次的状态。
func (r *Runner) decide(obs Observation) []Event {
	if obs.Kind == CollectOK {
		return r.onSuccess(obs)
	}
	return r.onFailure(obs)
}

func (r *Runner) onFailure(obs Observation) []Event {
	r.failStreak++
	var events []Event
	// 坏法变了就先记一条：非 200、坏 JSON、版本不认识都不该等到第三次才暴露。
	if obs.Kind != r.lastKind {
		events = append(events, Event{
			Type:     EventCollectError,
			Severity: "warning",
			Message:  r.failureText(obs),
		})
	}
	r.lastKind = obs.Kind
	if r.failStreak == failThreshold && !r.alerted {
		r.alerted = true
		events = append(events, Event{
			Type:     EventUnreachable,
			Severity: "error",
			Message:  fmt.Sprintf("%d consecutive collection failures: %s", r.failStreak, r.failureText(obs)),
		})
	}
	return events
}

func (r *Runner) onSuccess(obs Observation) []Event {
	var events []Event
	if r.alerted {
		r.alerted = false
		events = append(events, Event{
			Type:     EventRecovered,
			Severity: "info",
			Message:  "snapshot is readable again",
		})
	}
	if r.haveBase && (obs.Snapshot.Server.PID != r.pid || obs.Snapshot.Server.StartedAt != r.startedAt) {
		events = append(events, Event{
			Type:     EventRestarted,
			Severity: "warning",
			Message: fmt.Sprintf("server restarted: pid %d -> %d, started_at %q -> %q",
				r.pid, obs.Snapshot.Server.PID, r.startedAt, obs.Snapshot.Server.StartedAt),
		})
	}

	r.haveBase = true
	r.pid = obs.Snapshot.Server.PID
	r.startedAt = obs.Snapshot.Server.StartedAt
	r.failStreak = 0
	r.lastKind = CollectOK

	for i := range events {
		if events[i].Summary == nil {
			events[i].Summary = summaryOf(obs.Snapshot)
		}
	}
	return events
}

// failureText 把失败类别与 PID 文件的判断合成一句话，供状态文件与事件共用。
func (r *Runner) failureText(obs Observation) string {
	text := fmt.Sprintf("%s: %v", obs.Kind, obs.Err)
	if obs.Kind != FailUnreachable {
		return text
	}
	return text + "; " + r.serverVerdict()
}

// serverVerdict 在 /runtime 连不上时读 PID 文件，区分从未启动、已崩溃、启动中。
func (r *Runner) serverVerdict() string {
	pid, state, err := ReadPidFile(r.cfg.PidFile)
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

func (r *Runner) statusOf(obs Observation) MonitorStatus {
	st := MonitorStatus{
		Status:        StatusRunning,
		Version:       Version,
		PID:           os.Getpid(),
		Healthy:       true,
		LastCollectAt: obs.At.Format(TimeFormat),
		LastEventAt:   r.lastEventAt,
	}
	if obs.Kind != CollectOK {
		st.Status = StatusUnhealthy
		st.Healthy = false
		st.LastError = r.failureText(obs)
	}
	return st
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
