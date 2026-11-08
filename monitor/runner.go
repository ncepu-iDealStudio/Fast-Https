package main

import (
	"context"
	"log"
	"os"
	"time"
)

// Runner 驱动采集与落盘：采一次、交给规则引擎、写快照/事件/状态。
// 判定全在 Engine（rules.go），这里不重复任何规则。
type Runner struct {
	cfg    Config
	col    *Collector
	sink   *Sink
	clk    Clock
	logger *log.Logger
	engine *Engine

	lastEventAt string
}

// NewRunner 按配置组装采集器、规则引擎与落盘器。
// clk 与 logger 传 nil 时用系统时钟与标准错误。
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
		engine: newEngine(cfg, nil),
	}
}

// RunOnce 采一次、判一次、落一次盘。返回采集结果与本轮产生的事件。
func (r *Runner) RunOnce(ctx context.Context) (Observation, []Event) {
	obs := r.col.Collect(ctx, r.clk.Now())
	events := r.engine.Observe(obs)

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
		st.LastError = r.engine.FailureText(obs)
	}
	return st
}
