package cmd

import (
	"encoding/json"
	"fast-https/cmd/internal/servicecmd"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/proxy"
	"fast-https/runtime/metrics"
)

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
