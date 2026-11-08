// Package svcctl 受控 reload：校验、新增端口预检、发信号、弱验证。返回 error，不 Fatal。
package svcctl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"fast-https/config"
	"fast-https/management/state"
	"fast-https/management/view"
)

const DefaultTimeout = 30 * time.Second

var (
	ErrNotRunning = errors.New("service is not running")
	ErrTimeout    = errors.New("reload timed out")
)

// PortError 是新增端口被占用。
type PortError struct {
	Port int
}

func (e *PortError) Error() string {
	return fmt.Sprintf("port %d is in use", e.Port)
}

// Options 是一次 reload 的参数。Probe/Bind/Alive/Signal 可替换以便测试。
type Options struct {
	ConfigPath string
	PidPath    string
	Timeout    time.Duration
	DryRun     bool
	PID        int
	Probe      func(port int) bool
	Bind       func(port int) error
	Alive      func(pid int) bool
	Signal     func(pid int) error
	Sleep      func(d time.Duration)
}

// Verification 是弱验证结果。无法确认 Server 已应用新配置。
type Verification struct {
	Level           string `json:"level"`
	ReloadConfirmed bool   `json:"reload_confirmed"`
	ProcessAlive    bool   `json:"process_alive,omitempty"`
	PortsReachable  bool   `json:"ports_reachable,omitempty"`
}

// Result 是 Reload 的同步结果。
type Result struct {
	DryRun       bool         `json:"dry_run,omitempty"`
	PID          int          `json:"pid,omitempty"`
	Verification Verification `json:"verification"`
	AddedPorts   []int        `json:"added_ports,omitempty"`
}

// Reload 校验 → 新增端口预检 →（非 dry-run）发信号 → 弱验证。
func Reload(ctx context.Context, opts Options) (Result, error) {
	if err := config.ValidateConfigFile(opts.ConfigPath); err != nil {
		return Result{}, err
	}
	pid := opts.PID
	if pid <= 0 {
		var err error
		pid, err = readRunningPID(opts.PidPath, opts.Alive)
		if err != nil {
			return Result{}, err
		}
	}
	added, err := PrecheckAddedPorts(opts)
	if err != nil {
		return Result{}, err
	}
	out := Result{
		DryRun:     opts.DryRun,
		PID:        pid,
		AddedPorts: added,
		Verification: Verification{
			Level:           "basic",
			ReloadConfirmed: false,
		},
	}
	if opts.DryRun {
		return out, nil
	}
	sig := opts.Signal
	if sig == nil {
		sig = SignalReload
	}
	if err := sig(pid); err != nil {
		return out, err
	}
	ver, err := WaitBasic(ctx, opts, pid)
	out.Verification = ver
	return out, err
}

func readRunningPID(pidPath string, alive func(pid int) bool) (int, error) {
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	var m map[string]int
	if err := jsonUnmarshalPID(raw, &m); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	pid := m["pid"]
	check := alive
	if check == nil {
		check = func(pid int) bool { return state.DefaultAlive(pid, "") }
	}
	if !check(pid) {
		return 0, ErrNotRunning
	}
	return pid, nil
}

func jsonUnmarshalPID(raw []byte, m *map[string]int) error {
	return jsonUnmarshal(raw, m)
}

// PrecheckAddedPorts 对「新配置有、当前回环连不上」的端口试绑定。
func PrecheckAddedPorts(opts Options) ([]int, error) {
	ports, err := config.ListenPortsFromFile(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	probe := opts.Probe
	if probe == nil {
		probe = view.ProbeLoopback
	}
	bind := opts.Bind
	if bind == nil {
		bind = TryBind
	}
	var added []int
	for _, p := range ports {
		if probe(p) {
			continue
		}
		added = append(added, p)
		if err := bind(p); err != nil {
			var pe *PortError
			if errors.As(err, &pe) {
				return added, err
			}
			return added, &PortError{Port: p}
		}
	}
	return added, nil
}

// PrecheckTree 用候选目录的端口做同样预检（apply 在切换文件之前调用）。
func PrecheckTree(root string, probe func(int) bool, bind func(int) error) ([]int, error) {
	ports, err := config.ListenPortsFromTree(root)
	if err != nil {
		return nil, err
	}
	if probe == nil {
		probe = view.ProbeLoopback
	}
	if bind == nil {
		bind = TryBind
	}
	var added []int
	for _, p := range ports {
		if probe(p) {
			continue
		}
		added = append(added, p)
		if err := bind(p); err != nil {
			var pe *PortError
			if errors.As(err, &pe) {
				return added, err
			}
			return added, &PortError{Port: p}
		}
	}
	return added, nil
}

// TryBind 试着监听端口，成功则立刻关掉，把端口还给即将 reload 的进程。
func TryBind(port int) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return &PortError{Port: port}
	}
	_ = ln.Close()
	return nil
}

// WaitBasic 连续两次进程存活，且配置端口都能回环连通。
func WaitBasic(ctx context.Context, opts Options, pid int) (Verification, error) {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < timeout {
			timeout = d
		}
	}
	probe := opts.Probe
	if probe == nil {
		probe = view.ProbeLoopback
	}
	alive := opts.Alive
	if alive == nil {
		alive = func(pid int) bool { return state.DefaultAlive(pid, "") }
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	ports, err := config.ListenPortsFromFile(opts.ConfigPath)
	if err != nil {
		return Verification{Level: "basic", ReloadConfirmed: false}, err
	}

	deadline := time.Now().Add(timeout)
	var last Verification
	for {
		if err := ctx.Err(); err != nil {
			last.Level = "basic"
			return last, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		last = Verification{Level: "basic", ReloadConfirmed: false, ProcessAlive: alive(pid)}
		if last.ProcessAlive {
			sleep(50 * time.Millisecond)
			if alive(pid) {
				ok := true
				for _, p := range ports {
					if !probe(p) {
						ok = false
						break
					}
				}
				last.PortsReachable = ok || len(ports) == 0
				if last.PortsReachable {
					return last, nil
				}
			} else {
				last.ProcessAlive = false
			}
		}
		if !last.ProcessAlive {
			return last, fmt.Errorf("%w: process %d exited after reload signal", ErrNotRunning, pid)
		}
		if time.Now().After(deadline) {
			return last, ErrTimeout
		}
		sleep(100 * time.Millisecond)
	}
}
