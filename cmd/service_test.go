package cmd

import (
	"encoding/json"
	"fast-https/cmd/internal/procctl"
	"fast-https/cmd/internal/servicecmd"
	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/proxy"
	"fast-https/runtime/metrics"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ===== merged from foreground_test.go =====
func TestRunInForegroundFromEnv(t *testing.T) {
	t.Setenv("FASTHTTPS_FOREGROUND", "1")
	if !runInForeground() {
		t.Fatal("FASTHTTPS_FOREGROUND=1 should keep the server in the foreground")
	}
}

func TestRunInForegroundUnset(t *testing.T) {
	t.Setenv("FASTHTTPS_FOREGROUND", "")
	t.Setenv("INVOCATION_ID", "")
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("container marker exists in this environment")
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		t.Skip("container marker exists in this environment")
	}
	if runInForeground() {
		t.Fatal("empty FASTHTTPS_FOREGROUND should servicecmd.Daemonize outside a container")
	}
}

// ===== merged from register_test.go =====
func TestRegisterHandlersCoversConfiguredTypes(t *testing.T) {
	types := []struct {
		code uint16
		name string
	}{
		{config.LOCAL, "local"},
		{config.PROXY_HTTP, "proxy-http"},
		{config.PROXY_HTTPS, "proxy-https"},
		{config.REWRITE, "rewrite"},
		{config.DEVMOD, "devmod"},
	}
	for _, item := range types {
		core.RegisterHandler(int(item.code), nil, nil, nil)
	}
	t.Cleanup(servicecmd.RegisterHandlers)

	paths := make([]config.Path, len(types))
	for i, item := range types {
		paths[i].PathType = item.code
	}
	servers := []config.Server{{Path: paths}}

	err := core.MissingHandlers(servers)
	if err == nil {
		t.Fatal("unregistered location types should be reported before listening")
	}
	for _, item := range types {
		if !strings.Contains(err.Error(), item.name) {
			t.Errorf("error %q should name %s", err, item.name)
		}
	}

	servicecmd.RegisterHandlers()
	if err := core.MissingHandlers(servers); err != nil {
		t.Fatal(err)
	}
	for _, item := range types {
		if !core.HandlerReady(int(item.code)) {
			t.Errorf("%s handler is not ready after servicecmd.RegisterHandlers", item.name)
		}
	}
}

// ===== merged from runtime_endpoint_test.go =====
func startTestRuntime(t *testing.T) string {
	t.Helper()
	metrics.ResetForTest()
	servicecmd.StopRuntimeEndpoint()
	if err := servicecmd.StartRuntimeEndpointWith("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(servicecmd.StopRuntimeEndpoint)
	addr := servicecmd.RuntimeEndpointAddr()
	if addr == "" {
		t.Fatal("runtime endpoint did not bind")
	}
	url := "http://" + addr + "/runtime"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime endpoint never became ready")
	return ""
}

func getRuntime(t *testing.T, addr string) (int, []byte) {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

func TestRuntimeEndpointFields(t *testing.T) {
	addr := startTestRuntime(t)
	metrics.RecordProcessStart()
	metrics.MarkRunning()
	metrics.ConnAccepted()
	metrics.ConnClosed()
	metrics.RequestBegin()
	metrics.RequestEnd("HTTP/1.1 200 OK")
	metrics.ReplaceListeners([]metrics.ListenerInfo{{Port: "8080"}, {Port: "443", SSL: true, H2: true}})

	code, body := getRuntime(t, addr)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	var snap metrics.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		t.Fatalf("json: %v\n%s", err, body)
	}
	if snap.SchemaVersion != metrics.SchemaVersion {
		t.Fatalf("schema_version = %q", snap.SchemaVersion)
	}
	if snap.ObservedAt == "" {
		t.Fatal("observed_at missing")
	}
	if snap.Server.State != "running" {
		t.Fatalf("state = %q", snap.Server.State)
	}
	if snap.Server.PID != os.Getpid() {
		t.Fatalf("pid = %d", snap.Server.PID)
	}
	if snap.Server.Version != config.CURRENT_VERSION {
		t.Fatalf("version = %q", snap.Server.Version)
	}
	if snap.Connections.AcceptedTotal != 1 || snap.Connections.ClosedTotal != 1 {
		t.Fatalf("connections = %+v", snap.Connections)
	}
	if snap.Requests.Total != 1 || snap.Responses2xx != 1 {
		t.Fatalf("requests=%+v 2xx=%d", snap.Requests, snap.Responses2xx)
	}
	if len(snap.Listeners) != 2 || snap.Listeners[1].Port != "443" || !snap.Listeners[1].H2 {
		t.Fatalf("listeners = %+v", snap.Listeners)
	}
}

func TestRuntimeEndpointBindFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	err = servicecmd.StartRuntimeEndpointWith(ln.Addr().String())
	if err == nil {
		servicecmd.StopRuntimeEndpoint()
		t.Fatal("occupied address should fail to bind")
	}
	// 调用方可以继续：函数返回错误，没有把业务端口绑上。
}

func TestRuntimeEndpoint404(t *testing.T) {
	addr := startTestRuntime(t)
	resp, err := http.Get("http://" + addr + "/other")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRuntimeStartingState(t *testing.T) {
	metrics.ResetForTest()
	block := make(chan struct{})
	if err := servicecmd.StartRuntimeEndpointWith("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(servicecmd.StopRuntimeEndpoint)
	addr := servicecmd.RuntimeEndpointAddr()
	go func() {
		<-block // 假的 InitSystem：阻塞在 starting
		metrics.RecordProcessStart()
		metrics.MarkRunning()
	}()

	deadline := time.Now().Add(2 * time.Second)
	var body []byte
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/runtime")
		if err == nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(string(body), `"state":"starting"`) {
		t.Fatalf("during init, want starting, got %s", body)
	}
	close(block)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, body = getRuntime(t, addr)
		if strings.Contains(string(body), `"state":"running"`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("after ServerInit stand-in, want running, got %s", body)
}

func TestRuntimeClosedAfterStop(t *testing.T) {
	addr := startTestRuntime(t)
	core.SetServices(core.Services{
		AfterStop: servicecmd.StopRuntimeEndpoint,
	})
	t.Cleanup(func() { core.SetServices(core.Services{}) })
	core.RunAfterStop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := http.Get("http://" + addr + "/runtime")
		if err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("endpoint still accepting after AfterStop")
}

func TestRuntimeNoSensitiveWords(t *testing.T) {
	addr := startTestRuntime(t)
	metrics.RecordProcessStart()
	metrics.NoteRejected()
	_, body := getRuntime(t, addr)
	lower := strings.ToLower(string(body))
	for _, word := range []string{"blacklist", "ssl_certificate", "cookie", "authorization"} {
		if strings.Contains(lower, word) {
			t.Fatalf("response contains forbidden %q:\n%s", word, body)
		}
	}
}

func TestRuntimeLoopbackOnly(t *testing.T) {
	if servicecmd.RuntimeListenAddr != "127.0.0.1:9088" {
		t.Fatalf("listen addr = %q, want 127.0.0.1:9088", servicecmd.RuntimeListenAddr)
	}
	addr := startTestRuntime(t)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("bound host = %q, want 127.0.0.1 (not 0.0.0.0)", host)
	}
}

func TestRuntimeUpstreamSection(t *testing.T) {
	addr := startTestRuntime(t)
	_, body := getRuntime(t, addr)
	if strings.Contains(string(body), `"upstream"`) {
		t.Fatalf("upstream must be omitted until servicecmd.RegisterServices: %s", body)
	}

	metrics.RegisterSection("upstream", func() (any, bool) {
		return proxy.Snapshot()
	})
	_, body = getRuntime(t, addr)
	if !strings.Contains(string(body), `"dial_fail_total"`) {
		t.Fatalf("registered upstream missing: %s", body)
	}
}

// ===== merged from status_test.go =====
// writePidTo 写入的是调用方给的 pid（后台化时父进程写子进程的 pid），
// 不能像旧实现那样永远写自己的 pid。
func TestWritePidToRecordsGivenPid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := procctl.WritePidTo(path, 4321); err != nil {
		t.Fatal(err)
	}
	pid, err := procctl.ReadPid(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 4321 {
		t.Fatalf("pid = %d, want 4321", pid)
	}
}

// waitProcessGone 对活着的进程必须等满超时后返回 false。
func TestWaitProcessGoneTimesOut(t *testing.T) {
	start := time.Now()
	if procctl.WaitGone(os.Getpid(), 200*time.Millisecond) {
		t.Fatal("current process is alive, waitProcessGone must not report gone")
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("returned after %v, before the timeout", elapsed)
	}
}

func TestInspectProcessAbsent(t *testing.T) {
	state, pid, err := procctl.InspectProcess(filepath.Join(t.TempDir(), "missing.pid"))
	if err != nil {
		t.Fatal(err)
	}
	if state != procctl.ProcessAbsent || pid != 0 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := os.WriteFile(path, []byte(`{"pid":2147483646}`), 0o644); err != nil {
		t.Fatal(err)
	}
	state, pid, err := procctl.InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if procctl.Running(2147483646) {
		t.Skip("pid 2147483646 is in use on this machine")
	}
	if state != procctl.ProcessStale || pid != 2147483646 {
		t.Fatalf("state=%v pid=%d", state, pid)
	}
}

func TestInspectProcessRunning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fast-https.pid")
	if err := procctl.WritePid(path); err != nil {
		t.Fatal(err)
	}
	state, pid, err := procctl.InspectProcess(path)
	if err != nil {
		t.Fatal(err)
	}
	if state != procctl.ProcessRunning || pid != os.Getpid() {
		t.Fatalf("state=%v pid=%d, want running %d", state, pid, os.Getpid())
	}
}
