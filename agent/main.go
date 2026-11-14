package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fast-https/agent/model"
)

func main() {
	configPath := flag.String("config", "agent.json", "Agent 配置文件；不存在时按环境变量和默认值运行")
	monitorDir := flag.String("monitor-dir", "", "覆盖配置里的 Monitor 事件目录")
	stateDir := flag.String("state-dir", "", "覆盖配置里的 Agent 状态目录")
	once := flag.Bool("once", false, "读完当前事件文件后退出")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		os.Exit(1)
	}
	if *monitorDir != "" {
		cfg.MonitorDir = *monitorDir
	}
	if *stateDir != "" {
		cfg.StateDir = *stateDir
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *once {
		if err := runOnce(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
			os.Exit(1)
		}
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		err := runOnce(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	added, err := IngestOnce(cfg.MonitorDir, cfg.StateDir, now)
	beat(cfg, now, added.Added > 0, err)
	if err != nil {
		return err
	}
	if _, err := pollCerts(ctx, cfg.MonitorDir, cfg.StateDir, cfg.FastHTTPSBin, cfg.CertWarnDays, nil, now); err != nil {
		beat(cfg, now, false, err)
		return err
	}
	if _, err := CollectPending(cfg.StateDir, CollectOptions{
		Binary:     cfg.FastHTTPSBin,
		RuntimeURL: cfg.RuntimeURL,
		Now:        now,
		Ctx:        ctx,
	}); err != nil {
		beat(cfg, now, false, err)
		return err
	}
	_, err = DiagnosePending(cfg.StateDir, now, cfg.models())
	beat(cfg, now, false, err)
	return err
}

func beat(cfg Config, now time.Time, sawEvent bool, runErr error) {
	st, err := OpenStore(cfg.StateDir)
	if err != nil {
		return
	}
	defer st.Close()
	lastEvent := ""
	if sawEvent {
		lastEvent = now.UTC().Format(time.RFC3339)
	}
	msg := ""
	if runErr != nil {
		msg = runErr.Error()
	}
	_ = st.Beat(now, os.Getpid(), processStarted.UTC().Format(time.RFC3339), lastEvent, msg)
}

var processStarted = time.Now()

func (c Config) models() model.Provider {
	return model.FromEnv(c.ModelURL, c.Model, c.ModelExternalURL, c.ModelExternal, c.ModelKey)
}
