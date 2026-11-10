package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config 是 Agent 自己的配置，不读站点 fast-https.json。
// 优先级：命令行 > 配置文件 > 环境变量 > 默认值。
type Config struct {
	MonitorDir       string `json:"monitor_dir"`
	StateDir         string `json:"state_dir"`
	FastHTTPSBin     string `json:"fast_https_bin"`
	RuntimeURL       string `json:"runtime_url"`
	CertWarnDays     int    `json:"cert_warn_days"`
	ModelURL         string `json:"model_url"`
	Model            string `json:"model"`
	ModelExternalURL string `json:"model_external_url"`
	ModelExternal    string `json:"model_external"`
	ModelKey         string `json:"model_key"`
}

func defaultConfig() Config {
	return Config{
		MonitorDir:    "logs/monitor",
		StateDir:      "state/agent",
		FastHTTPSBin:  "fast-https",
		RuntimeURL:    "http://127.0.0.1:9088/runtime",
		CertWarnDays:  defaultCertWarnDays,
		Model:         "local",
		ModelExternal: "external",
	}
}

// LoadConfig 读取 agent.json。文件不存在时按环境变量和默认值运行。
func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	applyEnv(&cfg)
	if strings.TrimSpace(path) != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("read agent config %s: %w", path, err)
			}
		} else {
			var file Config
			if err := json.Unmarshal(raw, &file); err != nil {
				return Config{}, fmt.Errorf("parse agent config %s: %w", path, err)
			}
			mergeConfig(&cfg, file)
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_MONITOR_DIR")); v != "" {
		cfg.MonitorDir = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_DIR")); v != "" {
		cfg.StateDir = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_BIN")); v != "" {
		cfg.FastHTTPSBin = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_RUNTIME_URL")); v != "" {
		cfg.RuntimeURL = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_MODEL_URL")); v != "" {
		cfg.ModelURL = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_MODEL")); v != "" {
		cfg.Model = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_MODEL_EXTERNAL_URL")); v != "" {
		cfg.ModelExternalURL = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_MODEL_EXTERNAL")); v != "" {
		cfg.ModelExternal = v
	}
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_AGENT_MODEL_KEY")); v != "" {
		cfg.ModelKey = v
	}
}

func mergeConfig(cfg *Config, file Config) {
	if v := strings.TrimSpace(file.MonitorDir); v != "" {
		cfg.MonitorDir = v
	}
	if v := strings.TrimSpace(file.StateDir); v != "" {
		cfg.StateDir = v
	}
	if v := strings.TrimSpace(file.FastHTTPSBin); v != "" {
		cfg.FastHTTPSBin = v
	}
	if v := strings.TrimSpace(file.RuntimeURL); v != "" {
		cfg.RuntimeURL = v
	}
	if file.CertWarnDays > 0 {
		cfg.CertWarnDays = file.CertWarnDays
	}
	if v := strings.TrimSpace(file.ModelURL); v != "" {
		cfg.ModelURL = v
	}
	if v := strings.TrimSpace(file.Model); v != "" {
		cfg.Model = v
	}
	if v := strings.TrimSpace(file.ModelExternalURL); v != "" {
		cfg.ModelExternalURL = v
	}
	if v := strings.TrimSpace(file.ModelExternal); v != "" {
		cfg.ModelExternal = v
	}
	if v := strings.TrimSpace(file.ModelKey); v != "" {
		cfg.ModelKey = v
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.MonitorDir) == "" || strings.TrimSpace(c.StateDir) == "" {
		return fmt.Errorf("monitor_dir and state_dir must not be empty")
	}
	if c.CertWarnDays < 1 {
		return fmt.Errorf("cert_warn_days must be at least 1")
	}
	u, err := url.Parse(c.RuntimeURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid runtime_url %q", c.RuntimeURL)
	}
	return nil
}
