package cmd

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fast-https/config"
	"fast-https/infra/logging"
	"fast-https/management/operation"
	"fast-https/modules/certmanager"
)

// renewListenAddr 与 GET /runtime 分开。续期是写操作，不挂在只读快照上。
const renewListenAddr = "127.0.0.1:9089"

const renewCommandID = "cert.renew"

var (
	renewMu  sync.Mutex
	renewSrv *http.Server
	renewLn  net.Listener

	// renewDomain 是真正向 CA 续期的入口。测试换成不联网的替身。
	// 返回错误时不得替换现有证书文件。
	renewDomain = func(ctx context.Context, domain string) (renewStages, error) {
		if err := certmanager.Renew(ctx, domain); err != nil {
			return renewStages{}, err
		}
		return renewStages{Obtained: true, Written: true, Reloaded: true}, nil
	}

	renewClaims sync.Map
)

// renewStages 是响应里允许出现的全部字段。没有私钥，没有 PEM。
type renewStages struct {
	Obtained bool `json:"obtained"`
	Written  bool `json:"written"`
	Reloaded bool `json:"reloaded"`
}

type renewRequest struct {
	Domain    string `json:"domain"`
	RequestID string `json:"request_id"`
}

// StartRenewEndpoint 在 127.0.0.1:9089 提供 POST /renew。
// 每次调用都重写状态目录里的令牌文件（0600）。绑定失败只打日志。
func StartRenewEndpoint() error {
	if err := writeRenewToken(); err != nil {
		logging.ConsoleLog.Error().Msgf("renew endpoint token: %s", err.Error())
		return err
	}
	return startRenewEndpoint(renewListenAddr)
}

func startRenewEndpoint(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logging.ConsoleLog.Error().Msgf("renew endpoint listen %s failed: %s", addr, err.Error())
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/renew", serveRenew)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
	}
	renewMu.Lock()
	if renewSrv != nil {
		renewMu.Unlock()
		_ = ln.Close()
		_ = srv.Close()
		return nil
	}
	renewSrv = srv
	renewLn = ln
	renewMu.Unlock()

	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.ConsoleLog.Error().Msgf("renew endpoint serve: %s", err.Error())
		}
	}()
	return nil
}

// StopRenewEndpoint 关闭续期监听。停机时调用，不阻塞。
func StopRenewEndpoint() {
	renewMu.Lock()
	srv := renewSrv
	renewSrv = nil
	renewLn = nil
	renewMu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

func renewTokenPath() string {
	return filepath.Join(stateDir().Root, "renew.token")
}

func writeRenewToken() error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	path := renewTokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hex.EncodeToString(buf)+"\n"), 0o600)
}

func readRenewToken() (string, error) {
	b, err := os.ReadFile(renewTokenPath())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func serveRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !loopbackPeer(r.RemoteAddr) {
		writeRenew(w, http.StatusForbidden, renewStages{})
		return
	}
	token, err := readRenewToken()
	if err != nil || !bearerOK(r.Header.Get("Authorization"), token) {
		writeRenew(w, http.StatusUnauthorized, renewStages{})
		return
	}
	var body renewRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := dec.Decode(&body); err != nil {
		writeRenew(w, http.StatusBadRequest, renewStages{})
		return
	}
	domain := strings.TrimSpace(body.Domain)
	requestID := strings.TrimSpace(body.RequestID)
	if domain == "" || requestID == "" {
		writeRenew(w, http.StatusBadRequest, renewStages{})
		return
	}
	if !certmanager.Managed(config.Current(), domain) {
		logging.ConsoleLog.Info().Msgf("cert renew refused: %s is not an ACME-managed name", domain)
		writeRenew(w, http.StatusNotFound, renewStages{})
		return
	}
	store := operation.Store{Dir: stateDir()}
	rec, err := store.Begin(operation.BeginInput{
		RequestID: requestID,
		Command:   renewCommandID,
		Args:      map[string]string{"domain": domain},
		Phase:     "started",
	})
	if err != nil {
		if errors.Is(err, operation.ErrRequestIDConflict) {
			writeRenew(w, http.StatusConflict, renewStages{})
			return
		}
		logging.ConsoleLog.Error().Msgf("cert renew %s: operation begin failed", domain)
		writeRenew(w, http.StatusInternalServerError, renewStages{})
		return
	}
	if rec.Status == operation.StatusSucceeded {
		writeRenew(w, http.StatusOK, stagesFrom(rec.Result))
		return
	}
	if rec.Status == operation.StatusFailed {
		writeRenew(w, http.StatusInternalServerError, renewStages{})
		return
	}
	if !claimRenew(requestID, rec) {
		writeRenew(w, http.StatusConflict, renewStages{})
		return
	}
	stages, callErr := renewDomain(r.Context(), domain)
	if callErr != nil {
		// 不记录 callErr 的文本：续期失败原因可能夹带密钥材料。
		logging.ConsoleLog.Error().Msgf("cert renew %s failed, existing certificate unchanged", domain)
		_, _ = store.Finish(rec, operation.StatusFailed, "failed", nil, &operation.ErrorBody{
			Code:    "RENEW_FAILED",
			Message: "renew failed",
		})
		writeRenew(w, http.StatusInternalServerError, renewStages{})
		return
	}
	_, _ = store.Finish(rec, operation.StatusSucceeded, "reloaded", map[string]any{
		"obtained": stages.Obtained,
		"written":  stages.Written,
		"reloaded": stages.Reloaded,
	}, nil)
	logging.ConsoleLog.Info().Msgf("cert renew %s obtained=%t written=%t reloaded=%t",
		domain, stages.Obtained, stages.Written, stages.Reloaded)
	writeRenew(w, http.StatusOK, stages)
}

// claimRenew 只放行刚刚由本次调用创建的记录。已有的 running 记录不再向 CA 发第二次。
func claimRenew(requestID string, rec operation.Record) bool {
	if rec.Status != operation.StatusRunning {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, rec.CreatedAt)
	if err != nil || time.Since(created) > 2*time.Second {
		return false
	}
	_, loaded := renewClaims.LoadOrStore(requestID, struct{}{})
	return !loaded
}

func stagesFrom(result map[string]any) renewStages {
	return renewStages{
		Obtained: boolField(result, "obtained"),
		Written:  boolField(result, "written"),
		Reloaded: boolField(result, "reloaded"),
	}
}

func boolField(m map[string]any, key string) bool {
	v, ok := m[key].(bool)
	return ok && v
}

func writeRenew(w http.ResponseWriter, status int, stages renewStages) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(stages)
}

func loopbackPeer(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func bearerOK(header, token string) bool {
	const prefix = "Bearer "
	if token == "" || !strings.HasPrefix(header, prefix) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if len(got) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}
