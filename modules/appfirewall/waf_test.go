package appfirewall

import (
	"sync"
	"testing"

	"fast-https/config"
)

func cfgWith(rules []string, waf *config.WAFConfig) *config.Fast_Https {
	return &config.Fast_Https{
		Waf: waf,
		Servers: []config.Server{{
			Listen:     "80",
			ServerName: "a.local",
			Path:       []config.Path{{PathName: "/", AppFireWall: rules}},
		}},
	}
}

func TestWAFZeroCostDefault(t *testing.T) {
	if err := BuildWAF(cfgWith(nil, nil)); err != nil {
		t.Fatal(err)
	}
	defer ShutdownWAF()
	if current.Load() != nil {
		t.Fatal("engine must not be built without firewall config")
	}
	// 未配置 = 零开销放行
	if !Allowed(nil, testEvent("POST", "/login", "user=admin&pswd=' OR '1'='1")) {
		t.Error("nil engine must allow every request")
	}
}

// enforce 模式下 SQL 注入被阻断，正常请求放行
func TestWAFEnforceBlocksSQLi(t *testing.T) {
	if err := BuildWAF(cfgWith([]string{"sql"}, nil)); err != nil {
		t.Fatal(err)
	}
	defer ShutdownWAF()
	if current.Load() == nil {
		t.Fatal("engine should be built when appfirewall is configured")
	}

	if Allowed(nil, testEvent("POST", "/login", "user=admin&pswd=' OR '1'='1")) {
		t.Error("sqli payload should be blocked in enforce mode")
	}
	if !Allowed(nil, testEvent("POST", "/login", "user=admin&pswd=hello-world-2026")) {
		t.Error("normal request should pass")
	}
}

// monitor 模式只记录不阻断
func TestWAFMonitorDoesNotBlock(t *testing.T) {
	if err := BuildWAF(cfgWith([]string{"sql"}, &config.WAFConfig{Mode: "monitor"})); err != nil {
		t.Fatal(err)
	}
	defer ShutdownWAF()
	if Allowed(nil, testEvent("POST", "/login", "user=admin&pswd=' OR '1'='1")) {
		t.Error("monitor mode must not block")
	}
}

// disabled 等价于不启用
func TestWAFDisabledBuildsNothing(t *testing.T) {
	if err := BuildWAF(cfgWith([]string{"sql"}, &config.WAFConfig{Mode: "disabled"})); err != nil {
		t.Fatal(err)
	}
	defer ShutdownWAF()
	if current.Load() != nil {
		t.Error("disabled mode should not keep an engine")
	}
}

// xss 规则映射到 xss 检测器
func TestWAFBlocksXSS(t *testing.T) {
	if err := BuildWAF(cfgWith([]string{"xss"}, nil)); err != nil {
		t.Fatal(err)
	}
	defer ShutdownWAF()
	if Allowed(nil, testEvent("POST", "/comment", `{"name":"<script>alert(1)</script>"}`)) {
		t.Error("xss payload should be blocked")
	}
	if !Allowed(nil, testEvent("POST", "/comment", `{"name":"hello"}`)) {
		t.Error("plain json should pass")
	}
}

// reload：并发重建与检测（-race 验证原子切换）
func TestWAFBuildRace(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(2)
	stop := make(chan struct{})

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = BuildWAF(cfgWith([]string{"sql"}, nil))
		}
		close(stop)
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				Allowed(nil, testEvent("GET", "/", ""))
			}
		}
	}()
	wg.Wait()
	defer ShutdownWAF()
}
