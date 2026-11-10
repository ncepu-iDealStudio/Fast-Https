package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fast-https/config"
	"fast-https/runtime/listener"
)

var peerCursor struct {
	mu sync.Mutex
	n  map[string]uint64
}

func init() {
	peerCursor.n = map[string]uint64{}
}

func configuredPeers(cfg *listener.ListenCfg) []string {
	if cfg == nil {
		return nil
	}
	if len(cfg.ProxyPeers) > 0 {
		return append([]string(nil), cfg.ProxyPeers...)
	}
	if cfg.ProxyAddr != "" {
		return []string{cfg.ProxyAddr}
	}
	return nil
}

func activePeers(cfg *listener.ListenCfg) []string {
	drained := loadDrained()
	var out []string
	for _, peer := range configuredPeers(cfg) {
		if _, skip := drained[metricAddr(peer)]; skip {
			continue
		}
		out = append(out, peer)
	}
	return out
}

func rotatePeers(peers []string) []string {
	if len(peers) < 2 {
		return peers
	}
	key := strings.Join(peers, "\n")
	peerCursor.mu.Lock()
	i := peerCursor.n[key]
	peerCursor.n[key] = i + 1
	peerCursor.mu.Unlock()
	start := int(i % uint64(len(peers)))
	out := make([]string, 0, len(peers))
	out = append(out, peers[start:]...)
	out = append(out, peers[:start]...)
	return out
}

func NormalizePeer(addr string) string { return metricAddr(addr) }

func drainPath() string {
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_UPSTREAM_DRAIN_FILE")); v != "" {
		return v
	}
	return filepath.Join(config.FAST_HTTPS_STATE_DIR, "upstream-drained.json")
}

func loadDrained() map[string]struct{} {
	raw, err := os.ReadFile(drainPath())
	if err != nil {
		return map[string]struct{}{}
	}
	var file struct {
		Addresses []string `json:"addresses"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(file.Addresses))
	for _, addr := range file.Addresses {
		if host := metricAddr(addr); host != "" {
			out[host] = struct{}{}
		}
	}
	return out
}

// SaveDrained 原子替换摘除名单。名单不改 proxy_pass。
func SaveDrained(addrs []string) error {
	path := drainPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	clean := make([]string, 0, len(addrs))
	seen := map[string]struct{}{}
	for _, addr := range addrs {
		host := metricAddr(addr)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		clean = append(clean, host)
	}
	raw, err := json.Marshal(map[string]any{"addresses": clean})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DrainedList() []string {
	set := loadDrained()
	out := make([]string, 0, len(set))
	for addr := range set {
		out = append(out, addr)
	}
	return out
}
