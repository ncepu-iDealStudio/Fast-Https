package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"fast-https/config"
	"fast-https/infra/logging"
)

const renewFixtureKey = "FASTHTTPS-FIXTURE-PRIVATE-KEY-do-not-leak"

func useRenewState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FASTHTTPS_STATE_DIR", dir)
	return dir
}

func captureRenewLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logging.SetConsoleOutput(&buf)
	t.Cleanup(func() { logging.SetConsoleOutput(os.Stdout) })
	return &buf
}

func countRenewCalls(t *testing.T, succeed bool) *int {
	t.Helper()
	n := 0
	prev := renewDomain
	renewDomain = func(context.Context, string) (renewStages, error) {
		n++
		if !succeed {
			return renewStages{}, errors.New(renewFixtureKey)
		}
		return renewStages{Obtained: true, Written: true, Reloaded: true}, nil
	}
	t.Cleanup(func() { renewDomain = prev })
	return &n
}

func postRenew(remote, token, domain, requestID string) (int, string) {
	body := fmt.Sprintf(`{"domain":%q,"request_id":%q}`, domain, requestID)
	req := httptest.NewRequest(http.MethodPost, "/renew", strings.NewReader(body))
	req.RemoteAddr = remote
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	serveRenew(rr, req)
	return rr.Code, rr.Body.String()
}

func storeManagedConfig(t *testing.T) {
	t.Helper()
	prev := config.Current()
	t.Cleanup(func() { config.Store(prev, nil) })
	config.Store(&config.Fast_Https{Servers: []config.Server{{
		Listen:     "443 ssl",
		ServerName: "app.example",
	}}}, nil)
}

func stateDump(dir string) string {
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
		return nil
	})
	return b.String()
}

func TestRenewListenerIsSeparate(t *testing.T) {
	if renewListenAddr == runtimeListenAddr {
		t.Fatal("renew listener must not share the runtime address")
	}
}

func TestRenewTokenRewrittenEachStart(t *testing.T) {
	useRenewState(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(renewTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 0600", info.Mode().Perm())
	}
	first, err := os.ReadFile(renewTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(renewTokenPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("token was not regenerated")
	}
}

func TestRenewRejectsMissingToken(t *testing.T) {
	useRenewState(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, false)
	code, body := postRenew("127.0.0.1:9", "", "app.example", "missing-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d body %s", code, body)
	}
	if *calls != 0 {
		t.Fatalf("issuer calls = %d", *calls)
	}
}

func TestRenewRejectsWrongToken(t *testing.T) {
	useRenewState(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, false)
	code, body := postRenew("127.0.0.1:9", "not-the-token", "app.example", "wrong-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("status = %d body %s", code, body)
	}
	if *calls != 0 {
		t.Fatalf("issuer calls = %d", *calls)
	}
}

func TestRenewRejectsNonLoopback(t *testing.T) {
	useRenewState(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	token, err := readRenewToken()
	if err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, false)
	code, body := postRenew("192.0.2.8:9", token, "app.example", "non-loopback")
	if code != http.StatusForbidden {
		t.Fatalf("status = %d body %s", code, body)
	}
	if *calls != 0 {
		t.Fatalf("issuer calls = %d", *calls)
	}
}

func TestRenewDomainOutsidePlan(t *testing.T) {
	dir := useRenewState(t)
	storeManagedConfig(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	token, err := readRenewToken()
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(t.TempDir(), "live.pem")
	if err := os.WriteFile(certPath, []byte("OLD-CERT"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, false)
	code, body := postRenew("127.0.0.1:9", token, "other.example", "outside-plan")
	if code != http.StatusNotFound {
		t.Fatalf("status = %d body %s", code, body)
	}
	if *calls != 0 {
		t.Fatalf("issuer calls = %d", *calls)
	}
	got, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD-CERT" {
		t.Fatalf("certificate changed to %q", got)
	}
	if strings.Contains(body+stateDump(dir), renewFixtureKey) {
		t.Fatal("fixture key appeared before any issuer call")
	}
}

func TestRenewSameRequestIDCallsIssuerOnce(t *testing.T) {
	useRenewState(t)
	storeManagedConfig(t)
	logBuf := captureRenewLog(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	token, err := readRenewToken()
	if err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, true)
	code, body := postRenew("127.0.0.1:9", token, "app.example", "same-request")
	if code != http.StatusOK {
		t.Fatalf("first status = %d body %s", code, body)
	}
	code, again := postRenew("127.0.0.1:9", token, "app.example", "same-request")
	if code != http.StatusOK {
		t.Fatalf("second status = %d body %s", code, again)
	}
	if *calls != 1 {
		t.Fatalf("issuer calls = %d, want 1", *calls)
	}
	for _, text := range []string{body, again, logBuf.String()} {
		if strings.Contains(text, renewFixtureKey) || strings.Contains(text, "PRIVATE KEY") {
			t.Fatalf("secret appeared in output: %s", text)
		}
	}
}

func TestRenewFailureDoesNotReplaceCertificate(t *testing.T) {
	dir := useRenewState(t)
	storeManagedConfig(t)
	logBuf := captureRenewLog(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	token, err := readRenewToken()
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(t.TempDir(), "live.pem")
	if err := os.WriteFile(certPath, []byte("OLD-CERT"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := countRenewCalls(t, false)
	code, body := postRenew("127.0.0.1:9", token, "app.example", "renew-fails")
	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d body %s", code, body)
	}
	code, again := postRenew("127.0.0.1:9", token, "app.example", "renew-fails")
	if code != http.StatusInternalServerError {
		t.Fatalf("second status = %d body %s", code, again)
	}
	if *calls != 1 {
		t.Fatalf("issuer calls = %d, want 1", *calls)
	}
	got, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD-CERT" {
		t.Fatalf("certificate replaced with %q", got)
	}
	combined := body + again + logBuf.String() + stateDump(dir)
	if strings.Contains(combined, renewFixtureKey) {
		t.Fatal("fixture private key appeared in the response, log, or operation")
	}
}

func TestCertRenewRequiresYes(t *testing.T) {
	_, _, code := runCLI(t, "cert", "renew", "--domain", "app.example", "--request-id", "r1")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

func TestCertRenewCLIDoesNotPrintToken(t *testing.T) {
	useRenewState(t)
	if err := writeRenewToken(); err != nil {
		t.Fatal(err)
	}
	token, err := readRenewToken()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FASTHTTPS_RENEW_URL", "http://127.0.0.1:1/renew")
	out, errOut, code := runCLI(t, "cert", "renew", "--domain", "app.example", "--yes", "--request-id", "r-cli")
	if code != 4 {
		t.Fatalf("exit = %d stdout %s stderr %s", code, out, errOut)
	}
	if strings.Contains(out+errOut, token) {
		t.Fatal("token appeared in command output")
	}
}
