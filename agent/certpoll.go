package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCertWarnDays = 30

type certItem struct {
	Domains  []string `json:"domains"`
	File     string   `json:"file"`
	NotAfter string   `json:"not_after"`
	DaysLeft *int     `json:"days_left"`
}

// PollCerts 在 Monitor 还没写过同一张证书的过期事件时，用 cert list 补一条任务。
// CLI 失败只记审计，不编造证书状态。
func PollCerts(monitorDir, stateDir, binary string, warnDays int, run cmdRunner, now time.Time) (int, error) {
	if warnDays < 1 {
		warnDays = defaultCertWarnDays
	}
	if now.IsZero() {
		now = time.Now()
	}
	return pollCerts(context.Background(), monitorDir, stateDir, binary, warnDays, run, now)
}

func pollCerts(ctx context.Context, monitorDir, stateDir, binary string, warnDays int, run cmdRunner, now time.Time) (int, error) {
	if warnDays < 1 {
		warnDays = defaultCertWarnDays
	}
	if now.IsZero() {
		now = time.Now()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := runAllowed(ctx, run, binary, []string{"cert", "list", "--json"})
	st, sterr := OpenStore(stateDir)
	if sterr != nil {
		return 0, sterr
	}
	defer st.Close()
	if err != nil {
		return 0, st.Audit("cert_list_failed", err.Error(), now)
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Certificates []certItem `json:"certificates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || !envelope.OK {
		return 0, st.Audit("cert_list_failed", "cert list JSON is not ok", now)
	}
	added := 0
	for _, item := range envelope.Data.Certificates {
		if item.DaysLeft == nil {
			if err := st.Audit("cert_list_skipped", "days_left missing", now); err != nil {
				return added, err
			}
			continue
		}
		if *item.DaysLeft >= warnDays {
			continue
		}
		name := certName(item)
		if certAlreadyReported(monitorDir, name) {
			continue
		}
		id := certEventID(name, item.NotAfter)
		ev := lineEvent{
			Type:     "certificate_expiring",
			Severity: "warning",
			Message:  "certificate expiring: " + name + " days_left=" + itoa(*item.DaysLeft),
			EventID:  id,
		}
		ok, err := st.InsertEvent(ev, id, now)
		if err != nil {
			return added, err
		}
		if ok {
			added++
		}
	}
	return added, nil
}

func certName(item certItem) string {
	if len(item.Domains) > 0 && item.Domains[0] != "" {
		return item.Domains[0]
	}
	return filepath.Base(item.File)
}

func certEventID(name, notAfter string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + notAfter))
	return hex.EncodeToString(sum[:16])
}

func certAlreadyReported(monitorDir, name string) bool {
	raw, err := os.ReadFile(filepath.Join(monitorDir, eventsFile))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, `"certificate_expiring"`) {
			continue
		}
		if name != "" && strings.Contains(line, name) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	return strings.TrimPrefix(strings.ReplaceAll(jsonNumberText(n), " ", ""), "")
}

func jsonNumberText(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
