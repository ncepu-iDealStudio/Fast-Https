package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const cliTimeout = 5 * time.Second

// allowedCLI 是上下文采集唯一可以执行的参数表。每一项都是完整 argv，不含 Shell。
var allowedCLI = [][]string{
	{"service", "status", "--json"},
	{"service", "version", "--json"},
	{"runtime", "status", "--json"},
	{"runtime", "stats", "--json"},
	{"logs", "error", "--json", "--limit", "20"},
	{"config", "show", "--json"},
	{"config", "versions", "--json"},
	{"cert", "list", "--json"},
	{"diagnose", "--json"},
	{"operation", "list", "--json"},
	{"monitor", "status", "--json"},
}

// cmdRunner 执行一个 argv。生产实现是 exec.Command，测试注入替身。
type cmdRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// runAllowed 只执行白名单里的一项。参数必须整表相等，不能多一个 flag。
func runAllowed(ctx context.Context, run cmdRunner, binary string, args []string) (json.RawMessage, error) {
	if !cliAllowed(args) {
		return nil, fmt.Errorf("command not allow-listed")
	}
	if run == nil {
		run = execRunner
	}
	if binary == "" {
		binary = "fast-https"
	}
	cctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()
	out, err := run(cctx, binary, args...)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var body json.RawMessage
	if err := dec.Decode(&body); err != nil || len(body) == 0 || body[0] != '{' {
		return nil, fmt.Errorf("stdout is not one JSON object")
	}
	if dec.More() {
		return nil, fmt.Errorf("stdout is not one JSON object")
	}
	return body, nil
}

func cliAllowed(args []string) bool {
	for _, want := range allowedCLI {
		if len(want) != len(args) {
			continue
		}
		ok := true
		for i := range want {
			if want[i] != args[i] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
