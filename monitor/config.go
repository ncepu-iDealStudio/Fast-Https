package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultInterval 是默认采集周期，DefaultTimeout 是单次采集超时。
	DefaultInterval = 5 * time.Second
	DefaultTimeout  = 1 * time.Second

	// DefaultSnapshotURL 指向 Server 在组合根里起的本机只读接口。
	DefaultSnapshotURL = "http://127.0.0.1:9088/runtime"

	// DefaultDir 是状态与事件文件的默认目录，相对当前工作目录。
	DefaultDir = "logs/monitor"

	// DefaultPidFile 与站点进程写下的 PID 文件同名同格式，值取自配置基址。
	DefaultPidFile = "fast-https.pid"

	DefaultEventMaxBytes = int64(10 << 20)
	DefaultEventKeep     = 3

	// DefaultCooldown 是同一条规则触发后的冷却时间。
	DefaultCooldown = 5 * time.Minute

	// DefaultFiveXXRatio 是「5xx 升高」的默认阈值，DefaultFiveXXMinRequests
	// 是算比例前要求的最小请求增量，避免除以一个很小的数。
	DefaultFiveXXRatio       = 0.05
	DefaultFiveXXMinRequests = 20

	// 可选的两条提示规则默认开启：端口探测能发现「接口活着但业务端口不通」，
	// 卡死提示只是 info 级别。嫌吵可以在配置里显式写 false 关掉。
	DefaultPortProbe = true
	DefaultStuckHint = true

	// EnvDir 与 CLI 读状态文件时使用的环境变量一致。
	EnvDir = "FASTHTTPS_MONITOR_DIR"
)

// Duration 让配置文件里可以直接写 "5s"、"1s"，也接受以秒为单位的数字。
type Duration time.Duration

// D 取出标准库的 time.Duration。
func (d Duration) D() time.Duration { return time.Duration(d) }

// UnmarshalJSON 接受 "5s" 形式的字符串，或数字秒。
func (d *Duration) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return nil
	}
	if v, err := time.ParseDuration(strings.Trim(text, `"`)); err == nil {
		*d = Duration(v)
		return nil
	}
	var secs float64
	if err := json.Unmarshal(raw, &secs); err == nil {
		*d = Duration(time.Duration(secs * float64(time.Second)))
		return nil
	}
	return fmt.Errorf("invalid duration %s (want e.g. \"5s\")", text)
}

// Config 是 Monitor 自己的配置，独立于站点配置 fast-https.json。
// 它只决定采什么、多久采一次、往哪里写。
type Config struct {
	Interval      Duration `json:"interval"`
	Timeout       Duration `json:"timeout"`
	SnapshotURL   string   `json:"snapshot_url"`
	Dir           string   `json:"dir"`
	PidFile       string   `json:"pid_file"`
	EventMaxBytes int64    `json:"event_max_bytes"`
	EventKeep     int      `json:"event_keep"`

	// 规则口径。Cooldown 为 0 表示不做冷却。
	Cooldown          Duration `json:"cooldown"`
	FiveXXRatio       float64  `json:"five_xx_ratio"`
	FiveXXMinRequests int      `json:"five_xx_min_requests"`
	PortProbe         bool     `json:"port_probe"`
	StuckHint         bool     `json:"stuck_hint"`
}

// DefaultConfig 返回不带文件与环境变量影响的默认值。
func DefaultConfig() Config {
	return Config{
		Interval:      Duration(DefaultInterval),
		Timeout:       Duration(DefaultTimeout),
		SnapshotURL:   DefaultSnapshotURL,
		Dir:           DefaultDir,
		PidFile:       DefaultPidFile,
		EventMaxBytes: DefaultEventMaxBytes,
		EventKeep:     DefaultEventKeep,

		Cooldown:          Duration(DefaultCooldown),
		FiveXXRatio:       DefaultFiveXXRatio,
		FiveXXMinRequests: DefaultFiveXXMinRequests,
		PortProbe:         DefaultPortProbe,
		StuckHint:         DefaultStuckHint,
	}
}

// configFile 是配置文件的反序列化目标。两个可选规则用指针，
// 才能区分「没写」（用默认值，默认开）和「写了 false」（显式关掉）。
type configFile struct {
	Config
	PortProbe *bool `json:"port_probe"`
	StuckHint *bool `json:"stuck_hint"`
}

// LoadConfig 读取 Monitor 自己的配置。
//
// 文件不存在时按默认值运行；未知字段忽略，因此把站点配置误指过来也不会读进数据面配置。
// 优先级：配置文件 > 环境变量 FASTHTTPS_MONITOR_DIR > 默认值。
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	var file configFile
	if p := strings.TrimSpace(path); p != "" {
		raw, err := os.ReadFile(p)
		switch {
		case err == nil:
			if err := json.Unmarshal(raw, &file); err != nil {
				return Config{}, fmt.Errorf("parse monitor config %s: %w", p, err)
			}
		case os.IsNotExist(err):
			// 没有配置文件也能跑：部署脚本可以只靠命令行参数与默认值。
		default:
			return Config{}, fmt.Errorf("read monitor config %s: %w", p, err)
		}
	}
	mergeConfig(&cfg, file)
	if file.Dir == "" {
		if v := strings.TrimSpace(os.Getenv(EnvDir)); v != "" {
			cfg.Dir = v
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// mergeConfig 用配置文件里的非零字段覆盖默认值。
func mergeConfig(cfg *Config, file configFile) {
	if file.Interval != 0 {
		cfg.Interval = file.Interval
	}
	if file.Timeout != 0 {
		cfg.Timeout = file.Timeout
	}
	if v := strings.TrimSpace(file.SnapshotURL); v != "" {
		cfg.SnapshotURL = v
	}
	if v := strings.TrimSpace(file.Dir); v != "" {
		cfg.Dir = v
	}
	if v := strings.TrimSpace(file.PidFile); v != "" {
		cfg.PidFile = v
	}
	if file.EventMaxBytes > 0 {
		cfg.EventMaxBytes = file.EventMaxBytes
	}
	if file.EventKeep > 0 {
		cfg.EventKeep = file.EventKeep
	}
	if file.Cooldown != 0 {
		cfg.Cooldown = file.Cooldown
	}
	if file.FiveXXRatio > 0 {
		cfg.FiveXXRatio = file.FiveXXRatio
	}
	if file.FiveXXMinRequests > 0 {
		cfg.FiveXXMinRequests = file.FiveXXMinRequests
	}
	// 两个可选规则默认开：只有显式写了才改，写成 false 就是关掉。
	if file.PortProbe != nil {
		cfg.PortProbe = *file.PortProbe
	}
	if file.StuckHint != nil {
		cfg.StuckHint = *file.StuckHint
	}
}

// Validate 挡住会让采集空转的配置。它不检查目录可写：目录在首次落盘时才需要。
func (c Config) Validate() error {
	if c.Interval.D() < 100*time.Millisecond {
		return fmt.Errorf("interval %s is too small (minimum 100ms)", c.Interval.D())
	}
	if c.Timeout.D() <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if c.Timeout.D() > c.Interval.D() {
		return fmt.Errorf("timeout %s must not exceed interval %s", c.Timeout.D(), c.Interval.D())
	}
	if strings.TrimSpace(c.Dir) == "" {
		return fmt.Errorf("dir must not be empty")
	}
	if c.EventMaxBytes <= 0 {
		return fmt.Errorf("event_max_bytes must be positive")
	}
	if c.EventKeep < 1 {
		return fmt.Errorf("event_keep must be at least 1")
	}
	if c.Cooldown.D() < 0 {
		return fmt.Errorf("cooldown must not be negative")
	}
	if c.FiveXXRatio <= 0 || c.FiveXXRatio > 1 {
		return fmt.Errorf("five_xx_ratio must be in (0, 1], got %v", c.FiveXXRatio)
	}
	if c.FiveXXMinRequests < 1 {
		return fmt.Errorf("five_xx_min_requests must be at least 1")
	}
	u, err := url.Parse(c.SnapshotURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid snapshot_url %q", c.SnapshotURL)
	}
	return nil
}
