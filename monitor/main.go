// Package main 是独立的 Monitor 进程。
//
// 它周期读取 Server 的本机只读接口 GET /runtime，把原始快照追加到 JSONL，
// 并在连续采集失败、恢复、进程重启时写事件。
//
// 边界：Monitor 不 import fast-https 的数据面、管理核心与组合根，
// 不被 Server 启动，也不改动 Server 的任何状态。停掉 Monitor 不影响业务。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Version 是 Monitor 自身的版本，用 -ldflags 注入；不影响采集口径。
var Version = "dev"

func main() {
	logger := log.New(os.Stderr, "monitor: ", log.LstdFlags)
	cfgPath := flag.String("config", "config/monitor.json", "Monitor 配置文件；不存在时按默认值运行")
	dir := flag.String("dir", "", "状态与事件目录，覆盖配置文件与环境变量 "+EnvDir)
	once := flag.Bool("once", false, "只采集一次，便于手工验证")
	flag.Parse()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	if *dir != "" {
		cfg.Dir = *dir
	}
	if err := cfg.Validate(); err != nil {
		logger.Fatalf("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := NewRunner(cfg, SystemClock(), logger)
	if *once {
		obs, events := runner.RunOnce(ctx)
		fmt.Printf("collect=%s events=%d\n", obs.Kind, len(events))
		if obs.Kind != CollectOK {
			os.Exit(1)
		}
		return
	}

	logger.Printf("start interval=%s timeout=%s snapshot=%s dir=%s",
		cfg.Interval.D(), cfg.Timeout.D(), cfg.SnapshotURL, cfg.Dir)
	if err := runner.Run(ctx); err != nil {
		logger.Fatalf("%v", err)
	}
	logger.Printf("stopped")
}
