package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// certChecker 只读配置里列出的 PEM 文件，不读站点配置。
// 剩余天数低于阈值时写 certificate_expiring。同一文件、同一 NotAfter
// 在冷却期内只写一次。读失败或解析失败写一条 warning，不 panic。
type certChecker struct {
	files    []string
	warnDays int
	cooldown time.Duration
	last     map[string]time.Time
}

func newCertChecker(cfg Config) *certChecker {
	days := cfg.CertWarnDays
	if days < 1 {
		days = DefaultCertWarnDays
	}
	return &certChecker{
		files:    append([]string(nil), cfg.CertFiles...),
		warnDays: days,
		cooldown: cfg.Cooldown.D(),
		last:     map[string]time.Time{},
	}
}

// Check 在 now 这一刻检查全部证书。files 为空时不产生事件。
func (c *certChecker) Check(now time.Time) []Event {
	if c == nil || len(c.files) == 0 {
		return nil
	}
	var events []Event
	for _, path := range c.files {
		ev, ok := c.checkFile(now, path)
		if ok {
			events = append(events, ev)
		}
	}
	return events
}

func (c *certChecker) checkFile(now time.Time, path string) (Event, bool) {
	name := filepath.Base(path)
	notAfter, err := earliestNotAfter(path)
	if err != nil {
		return c.emit(now, "read\x00"+path, EventCertRead, "warning",
			fmt.Sprintf("certificate unreadable: %s", name))
	}
	days := daysLeft(notAfter, now)
	if days >= c.warnDays {
		return Event{}, false
	}
	msg := fmt.Sprintf("certificate expiring: %s days_left=%d", name, days)
	key := path + "\x00" + notAfter.UTC().Format(time.RFC3339Nano)
	return c.emit(now, key, EventCertExpiring, "warning", msg)
}

func (c *certChecker) emit(now time.Time, key, kind, severity, message string) (Event, bool) {
	if c.cooldown > 0 {
		if last, ok := c.last[key]; ok && now.Sub(last) < c.cooldown {
			return Event{}, false
		}
	}
	c.last[key] = now
	return Event{
		Time:     now.Format(TimeFormat),
		Type:     kind,
		Rule:     kind,
		Severity: severity,
		Message:  message,
	}, true
}

// earliestNotAfter 取文件里最早到期的那张证书。链上的叶子通常短于 CA，
// 用最早的到期日避免只看见后面那张长期证书。
func earliestNotAfter(path string) (time.Time, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	var earliest time.Time
	found := false
	rest := raw
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		if !found || cert.NotAfter.Before(earliest) {
			earliest = cert.NotAfter
			found = true
		}
	}
	if !found {
		return time.Time{}, fmt.Errorf("no certificate")
	}
	return earliest, nil
}

func daysLeft(notAfter, now time.Time) int {
	return int(notAfter.Sub(now) / (24 * time.Hour))
}
