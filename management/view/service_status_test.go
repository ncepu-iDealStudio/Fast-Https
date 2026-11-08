package view

import (
	"errors"
	"net"
	"testing"
	"time"
)

func fixedNow() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }

func inspectOf(state ProcState, pid int, err error) func() (ProcState, int, error) {
	return func() (ProcState, int, error) { return state, pid, err }
}

func listenLoopback(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestServiceStatusMatrix(t *testing.T) {
	up, down := listenLoopback(t), closedPort(t)

	cases := []struct {
		name       string
		in         ServiceStatusInput
		want       string
		wantStale  bool
		wantPID    bool
		wantWarnAt int // 期望的告警条数
	}{
		{"running", ServiceStatusInput{Inspect: inspectOf(ProcRunning, 42, nil), Ports: []int{up, down}}, "running", false, true, 0},
		{"degraded", ServiceStatusInput{Inspect: inspectOf(ProcRunning, 42, nil), Ports: []int{down}}, "degraded", false, true, 0},
		{"stopped absent", ServiceStatusInput{Inspect: inspectOf(ProcAbsent, 0, nil), Ports: []int{down}}, "stopped", false, false, 0},
		{"stopped stale", ServiceStatusInput{Inspect: inspectOf(ProcStale, 99, nil), Ports: []int{down}}, "stopped", true, false, 0},
		{"port up without process", ServiceStatusInput{Inspect: inspectOf(ProcAbsent, 0, nil), Ports: []int{up}}, "unknown", false, false, 1},
		{"pid unreadable", ServiceStatusInput{Inspect: inspectOf(ProcAbsent, 0, errors.New("boom")), Ports: []int{down}}, "unknown", false, false, 1},
		{"running without port info", ServiceStatusInput{Inspect: inspectOf(ProcRunning, 42, nil)}, "running", false, true, 1},
		{"running, ports unavailable", ServiceStatusInput{Inspect: inspectOf(ProcRunning, 42, nil), PortsErr: errors.New("no config")}, "running", false, true, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.in.Now = fixedNow
			st, warnings := ServiceStatusOf(c.in)
			if st.Status != c.want {
				t.Fatalf("status = %q, want %q", st.Status, c.want)
			}
			if st.StalePID != c.wantStale {
				t.Errorf("stale_pid = %v, want %v", st.StalePID, c.wantStale)
			}
			if (st.PID != nil) != c.wantPID {
				t.Errorf("pid present = %v, want %v", st.PID != nil, c.wantPID)
			}
			if len(warnings) != c.wantWarnAt {
				t.Errorf("warnings = %v, want %d", warnings, c.wantWarnAt)
			}
			if st.ObservedAt != "2026-11-08T12:00:00Z" || st.Source == "" || st.Ports == nil {
				t.Errorf("基础字段不对：%+v", st)
			}
		})
	}
}

func TestProbeLoopback(t *testing.T) {
	if !ProbeLoopback(listenLoopback(t)) {
		t.Fatal("监听中的端口应可达")
	}
	if ProbeLoopback(closedPort(t)) {
		t.Fatal("已关闭的端口不应可达")
	}
}
