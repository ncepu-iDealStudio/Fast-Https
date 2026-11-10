package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	monitorDir := flag.String("monitor-dir", "", "Monitor 事件目录，默认同 FASTHTTPS_MONITOR_DIR，再没有则用 logs/monitor")
	stateDir := flag.String("state-dir", "", "Agent 状态目录，默认同 FASTHTTPS_AGENT_DIR，再没有则用 ./state/agent")
	once := flag.Bool("once", false, "读完当前事件文件后退出")
	flag.Parse()

	mon := firstNonEmpty(*monitorDir, os.Getenv("FASTHTTPS_MONITOR_DIR"), "logs/monitor")
	state := firstNonEmpty(*stateDir, os.Getenv("FASTHTTPS_AGENT_DIR"), "state/agent")
	if *once {
		if err := runOnce(mon, state); err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
			os.Exit(1)
		}
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(mon, state); err != nil {
			fmt.Fprintf(os.Stderr, "agent: %v\n", err)
		}
		<-ticker.C
	}
}

func runOnce(mon, state string) error {
	now := time.Now()
	if _, err := IngestOnce(mon, state, now); err != nil {
		return err
	}
	if _, err := PollCerts(mon, state, defaultCertWarnDays, nil, now); err != nil {
		return err
	}
	if _, err := CollectPending(state, CollectOptions{Now: now}); err != nil {
		return err
	}
	_, err := DiagnosePending(state, now)
	return err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
