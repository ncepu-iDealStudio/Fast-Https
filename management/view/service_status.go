package view

import (
	"net"
	"strconv"
	"time"
)

// ProcState 是 pid 文件与进程探测的结果，与 cmd.ProcessState 一一对应。
type ProcState int

const (
	ProcAbsent  ProcState = iota // 没有 pid 文件
	ProcStale                    // pid 文件在，进程已不在
	ProcRunning                  // pid 文件在，进程还在
)

// PortStatus 是一个配置端口的回环连通结果。
type PortStatus struct {
	Port      int  `json:"port"`
	Reachable bool `json:"reachable"`
}

// ServiceStatus 是 service status 的数据。拿不到的字段省略，不编造。
type ServiceStatus struct {
	Status        string       `json:"status"`
	PID           *int         `json:"pid,omitempty"`
	StalePID      bool         `json:"stale_pid,omitempty"`
	ObservedAt    string       `json:"observed_at"`
	Source        string       `json:"source"`
	Ports         []PortStatus `json:"ports"`
	UptimeSeconds *int64       `json:"uptime_seconds,omitempty"`
	StartedAt     string       `json:"started_at,omitempty"`
	ConfigVersion string       `json:"config_version,omitempty"`
	Version       string       `json:"version,omitempty"`
}

// ServiceStatusInput 把外部依赖都做成可替换的函数，便于测试。
type ServiceStatusInput struct {
	Inspect  func() (ProcState, int, error)
	Ports    []int
	PortsErr error
	Probe    func(port int) bool
	Now      func() time.Time
}

// ProbeLoopback 对 127.0.0.1 上的端口做一次 TCP 连接，成功即视为可达。
func ProbeLoopback(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ServiceStatusOf 判定服务状态：
//
//	running   进程在，且至少一个配置端口可连（没有端口信息时只凭进程判断并给出告警）
//	degraded  进程在，但没有任何配置端口可连
//	stopped   没有进程，也没有端口可连（stale_pid 标明 pid 文件是残留的）
//	unknown   证据互相矛盾或读不出：例如没有进程却有端口可连
func ServiceStatusOf(in ServiceStatusInput) (ServiceStatus, []string) {
	probe := in.Probe
	if probe == nil {
		probe = ProbeLoopback
	}
	now := in.Now
	if now == nil {
		now = time.Now
	}

	st := ServiceStatus{
		ObservedAt: now().Format(time.RFC3339),
		Source:     "pid_file+port_probe",
		Ports:      []PortStatus{},
	}
	var warnings []string

	reachable := false
	for _, port := range in.Ports {
		ok := probe(port)
		reachable = reachable || ok
		st.Ports = append(st.Ports, PortStatus{Port: port, Reachable: ok})
	}
	if in.PortsErr != nil {
		warnings = append(warnings, "configured ports unavailable: "+in.PortsErr.Error())
	}

	state, pid, err := in.Inspect()
	switch {
	case err != nil:
		st.Status = "unknown"
		warnings = append(warnings, "pid file unreadable: "+err.Error())
	case state == ProcRunning:
		st.PID = &pid
		switch {
		case len(in.Ports) == 0:
			st.Status = "running"
			if in.PortsErr == nil {
				warnings = append(warnings, "no listen port is configured; cannot confirm the server accepts connections")
			}
		case reachable:
			st.Status = "running"
		default:
			st.Status = "degraded"
		}
	default:
		st.StalePID = state == ProcStale
		if reachable {
			st.Status = "unknown"
			warnings = append(warnings,
				"a configured port is reachable but no live process matches the pid file; "+
					"another process may be listening, or the pid file is missing")
		} else {
			st.Status = "stopped"
		}
	}
	return st, warnings
}
