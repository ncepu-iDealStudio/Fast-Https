package main

import (
	"flag"
	"fmt"
	"os"
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
	if *once {
		if err := runOnce(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
			os.Exit(1)
		}
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		}
		<-ticker.C
	}
}

func runOnce(cfg Config) error {
	now := time.Now()
	if _, err := IngestOnce(cfg.MonitorDir, cfg.StateDir, now); err != nil {
		return err
	}
	if _, err := PollCerts(cfg.MonitorDir, cfg.StateDir, cfg.FastHTTPSBin, cfg.CertWarnDays, nil, now); err != nil {
		return err
	}
	if _, err := CollectPending(cfg.StateDir, CollectOptions{
		Binary:     cfg.FastHTTPSBin,
		RuntimeURL: cfg.RuntimeURL,
		Now:        now,
	}); err != nil {
		return err
	}
	_, err := DiagnosePending(cfg.StateDir, now, cfg.models())
	return err
}

func (c Config) models() model.Provider {
	return model.FromEnv(c.ModelURL, c.Model, c.ModelExternalURL, c.ModelExternal, c.ModelKey)
}
