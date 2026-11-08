// Package diagnose 汇总一组只读健康检查。
//
// 检查失败（error/warning）和检查不可用（unavailable）要分开：
// 后者表示这项现在还没有数据源，不拉低 overall。
// 不写报告文件，不自动修复，输出里不带口令、密钥或日志原文。
package diagnose

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/management/logquery"
	"fast-https/management/view"
)

const (
	StatusOK          = "ok"
	StatusWarning     = "warning"
	StatusError       = "error"
	StatusUnavailable = "unavailable"

	defaultTimeout = 3 * time.Second
)

// Check 是一项检查的结果。
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// Report 是一次诊断的汇总。命令退出码仍为 0：这是一次成功的查询。
type Report struct {
	ObservedAt string  `json:"observed_at"`
	Overall    string  `json:"overall"`
	Checks     []Check `json:"checks"`
	ReportID   string  `json:"report_id"`
}

// NamedCheck 是可注入的检查，测试用它制造超时或替身。
type NamedCheck struct {
	Name string
	Fn   func(context.Context) Check
}

// Input 把外部依赖做成可替换的函数。命令层填真实实现，测试填替身。
type Input struct {
	ConfigPath string
	PidFile    string
	Now        func() time.Time
	Timeout    time.Duration
	Inspect    func() (view.ProcState, int, error)
	Ports      func() ([]int, error)
	Probe      func(int) bool
	StateDir   string
	Extra      []NamedCheck
}

// Run 按固定顺序执行检查。一项卡住只让那一项超时，其余照常返回。
func Run(in Input) Report {
	now := time.Now()
	if in.Now != nil {
		now = in.Now()
	}
	timeout := in.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	checks := []Check{
		timed(timeout, "service", func(ctx context.Context) Check { return checkService(in) }),
		timed(timeout, "config", func(ctx context.Context) Check { return checkConfig(in) }),
		timed(timeout, "config_drift", func(ctx context.Context) Check { return checkDrift(in) }),
		timed(timeout, "certs", func(ctx context.Context) Check { return checkCerts(in, now) }),
		timed(timeout, "ports", func(ctx context.Context) Check { return checkPorts(in) }),
		timed(timeout, "logs", func(ctx context.Context) Check { return checkLogs(in, now) }),
		{Name: "runtime", Status: StatusUnavailable, Message: "runtime snapshot is not available until S11"},
		{Name: "monitor", Status: StatusUnavailable, Message: "monitor is not available until S12"},
		timed(timeout, "state", func(ctx context.Context) Check { return checkState(in) }),
	}
	for _, extra := range in.Extra {
		if extra.Fn == nil {
			continue
		}
		checks = append(checks, timed(timeout, extra.Name, extra.Fn))
	}

	return Report{
		ObservedAt: now.UTC().Format(time.RFC3339),
		Overall:    overallOf(checks),
		Checks:     checks,
		ReportID:   newReportID(now),
	}
}

func timed(timeout time.Duration, name string, fn func(context.Context) Check) Check {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ch := make(chan Check, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				ch <- Check{Name: name, Status: StatusError, Message: "check panicked"}
			}
		}()
		c := fn(ctx)
		if c.Name == "" {
			c.Name = name
		}
		ch <- c
	}()
	select {
	case c := <-ch:
		return c
	case <-ctx.Done():
		return Check{Name: name, Status: StatusUnavailable, Message: "check timed out"}
	}
}

func overallOf(checks []Check) string {
	overall := StatusOK
	for _, c := range checks {
		switch c.Status {
		case StatusError:
			return StatusError
		case StatusWarning:
			overall = StatusWarning
		}
	}
	return overall
}

func newReportID(now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("diag_%s_%x", now.UTC().Format("20060102T150405Z"), b)
}

func checkService(in Input) Check {
	st, _ := view.ServiceStatusOf(view.ServiceStatusInput{
		Inspect:  inspectOf(in),
		Ports:    portsOf(in),
		PortsErr: portsErrOf(in),
		Probe:    probeOf(in),
		Now:      in.Now,
	})
	c := Check{Name: "service"}
	switch st.Status {
	case "running":
		c.Status = StatusOK
		if st.PID != nil {
			c.Message = fmt.Sprintf("service is running (pid %d)", *st.PID)
		} else {
			c.Message = "service is running"
		}
	case "degraded":
		c.Status = StatusWarning
		c.Message = "process is alive but no configured port is reachable"
	case "stopped":
		c.Status = StatusWarning
		c.Message = "service is not running"
	default:
		c.Status = StatusWarning
		c.Message = "service status is unknown"
	}
	return c
}

func checkConfig(in Input) Check {
	_, _, err := view.ConfigValidate(in.ConfigPath, false)
	if err == nil {
		return Check{Name: "config", Status: StatusOK, Message: "configuration is valid"}
	}
	msg := "configuration is invalid"
	var invalid *view.InvalidError
	switch {
	case errors.As(err, &invalid):
		msg = "configuration is invalid"
	case view.IsNotFound(err):
		msg = "configuration file not found"
	}
	return Check{Name: "config", Status: StatusError, Message: msg}
}

func checkDrift(in Input) Check {
	state, _, err := inspectOf(in)()
	if err != nil || state != view.ProcRunning {
		return Check{Name: "config_drift", Status: StatusUnavailable, Message: "no running process to compare against"}
	}
	dirty, known, err := view.ConfigDrift(in.ConfigPath, in.PidFile)
	if err != nil {
		return Check{Name: "config_drift", Status: StatusWarning, Message: "could not compare config files with the running process"}
	}
	if !known {
		return Check{Name: "config_drift", Status: StatusUnavailable, Message: "no pid file to compare against"}
	}
	if dirty {
		return Check{Name: "config_drift", Status: StatusWarning, Message: "config files are newer than the pid file (hint)"}
	}
	return Check{Name: "config_drift", Status: StatusOK, Message: "on-disk config is not newer than the pid file"}
}

func checkCerts(in Input, now time.Time) Check {
	data, _, err := view.CertList(in.ConfigPath, now)
	if err != nil {
		if view.IsNotFound(err) {
			return Check{Name: "certs", Status: StatusError, Message: "configuration file not found"}
		}
		return Check{Name: "certs", Status: StatusError, Message: "could not read certificate declarations"}
	}
	if len(data.Certificates) == 0 {
		return Check{Name: "certs", Status: StatusOK, Message: "no ssl server block is configured"}
	}
	var expired, missing, unreadable, expiring, unknown int
	for _, e := range data.Certificates {
		switch e.Status {
		case view.CertExpired:
			expired++
		case view.CertMissing:
			missing++
		case view.CertUnreadable:
			unreadable++
		case view.CertExpiring:
			expiring++
		case view.CertUnknown, view.CertNotYetValid:
			unknown++
		}
	}
	bad := expired + missing + unreadable
	if bad > 0 {
		return Check{Name: "certs", Status: StatusError, Message: fmt.Sprintf("%d certificate file(s) expired, missing, or unreadable", bad)}
	}
	if expiring > 0 {
		return Check{Name: "certs", Status: StatusWarning, Message: fmt.Sprintf("%d certificate file(s) expiring", expiring)}
	}
	if unknown > 0 {
		return Check{Name: "certs", Status: StatusWarning, Message: fmt.Sprintf("%d certificate file(s) not yet valid or unknown", unknown)}
	}
	return Check{Name: "certs", Status: StatusOK, Message: fmt.Sprintf("%d certificate file(s) valid", len(data.Certificates))}
}

func checkPorts(in Input) Check {
	ports, err := portsResult(in)
	if err != nil {
		return Check{Name: "ports", Status: StatusWarning, Message: "could not read listen ports from the config"}
	}
	if len(ports) == 0 {
		return Check{Name: "ports", Status: StatusWarning, Message: "no listen port is configured"}
	}
	probe := probeOf(in)
	reachable := 0
	for _, p := range ports {
		if probe(p) {
			reachable++
		}
	}
	switch {
	case reachable == len(ports):
		return Check{Name: "ports", Status: StatusOK, Message: fmt.Sprintf("all %d configured port(s) reachable", len(ports))}
	case reachable == 0:
		return Check{Name: "ports", Status: StatusWarning, Message: fmt.Sprintf("none of %d configured port(s) reachable", len(ports))}
	default:
		return Check{Name: "ports", Status: StatusWarning, Message: fmt.Sprintf("%d of %d configured port(s) reachable", reachable, len(ports))}
	}
}

func checkLogs(in Input, now time.Time) Check {
	dir, _ := logquery.ResolveLogDir(in.ConfigPath)
	res, _, err := logquery.Run(logquery.Query{
		Dir:   dir,
		Kind:  logging.KindError,
		Since: "1h",
		Limit: 100,
		Now:   now,
	})
	if err != nil {
		var src *logquery.SourceError
		if errors.As(err, &src) {
			return Check{Name: "logs", Status: StatusUnavailable, Message: "error.log is not available"}
		}
		if os.IsPermission(err) || errors.Is(err, fs.ErrPermission) {
			return Check{Name: "logs", Status: StatusError, Message: "error.log is unreadable"}
		}
		return Check{Name: "logs", Status: StatusWarning, Message: "could not read error.log"}
	}
	n := len(res.Records)
	msg := fmt.Sprintf("%d error record(s) in the last hour", n)
	if res.Truncated {
		msg += " (truncated)"
	}
	if n == 0 {
		return Check{Name: "logs", Status: StatusOK, Message: msg}
	}
	return Check{Name: "logs", Status: StatusWarning, Message: msg}
}

func checkState(in Input) Check {
	dir := strings.TrimSpace(in.StateDir)
	if dir == "" {
		return Check{Name: "state", Status: StatusUnavailable, Message: "state directory is not configured"}
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Check{Name: "state", Status: StatusWarning, Message: "state directory does not exist yet"}
		}
		return Check{Name: "state", Status: StatusError, Message: "state directory is not accessible"}
	}
	if !info.IsDir() {
		return Check{Name: "state", Status: StatusError, Message: "state path is not a directory"}
	}
	return Check{Name: "state", Status: StatusOK, Message: "state directory is present"}
}

func inspectOf(in Input) func() (view.ProcState, int, error) {
	if in.Inspect != nil {
		return in.Inspect
	}
	return func() (view.ProcState, int, error) { return view.ProcAbsent, 0, nil }
}

func probeOf(in Input) func(int) bool {
	if in.Probe != nil {
		return in.Probe
	}
	return view.ProbeLoopback
}

func portsResult(in Input) ([]int, error) {
	if in.Ports != nil {
		return in.Ports()
	}
	if in.ConfigPath == "" {
		return nil, nil
	}
	return config.ListenPortsFromFile(in.ConfigPath)
}

func portsOf(in Input) []int {
	ports, _ := portsResult(in)
	return ports
}

func portsErrOf(in Input) error {
	_, err := portsResult(in)
	return err
}
