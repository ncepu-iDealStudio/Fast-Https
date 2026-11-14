package packaging

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGoreleaserShipsAgent(t *testing.T) {
	yaml := readPkgFile(t, "build/goreleaser.yaml")
	if n := strings.Count(yaml, "main: ./agent"); n < 3 {
		t.Errorf("builds 里只有 %d 处 main: ./agent，期望至少 3 处", n)
	}
	if n := strings.Count(yaml, "- agent-linux-pkg"); n != nfpmEntries {
		t.Errorf("nfpms 里有 %d 处 agent-linux-pkg，期望 %d", n, nfpmEntries)
	}
	if n := strings.Count(yaml, "dst: /usr/lib/systemd/system/fast-https-agent.service"); n != nfpmEntries {
		t.Errorf("安装 Agent 单元 %d 处，期望 %d", n, nfpmEntries)
	}
	conffile := "dst: /etc/fast-https/agent.json\n        type: config|noreplace"
	if n := strings.Count(yaml, conffile); n != nfpmEntries {
		t.Errorf("agent.json 只有 %d 处 config|noreplace，期望 %d", n, nfpmEntries)
	}
	for _, want := range []string{"- agent-unix", "- agent-windows"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("archives 缺少 %q", want)
		}
	}
}

func TestAgentUnitIsIndependentOfServer(t *testing.T) {
	unit := readPkgFile(t, "build/packaging/systemd/fast-https-agent.service")
	for _, want := range []string{
		"ExecStart=/usr/bin/fast-https-agent -config /etc/fast-https/agent.json",
		"StateDirectory=fast-https",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("Agent 单元缺少 %q", want)
		}
	}
	for _, banned := range []string{"Requires=fast-https", "BindsTo=fast-https", "PartOf=fast-https"} {
		if strings.Contains(unit, banned) {
			t.Errorf("Agent 单元不应包含 %q", banned)
		}
	}
}

func TestPackagedAgentConfigUsesFHSPaths(t *testing.T) {
	raw := readPkgFile(t, "build/packaging/agent.json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("agent.json: %v", err)
	}
	if cfg["monitor_dir"] != "/var/log/fast-https/monitor" {
		t.Errorf("monitor_dir = %v", cfg["monitor_dir"])
	}
	if cfg["state_dir"] != "/var/lib/fast-https/agent" {
		t.Errorf("state_dir = %v", cfg["state_dir"])
	}
	if cfg["fast_https_bin"] != "/usr/bin/fast-https" {
		t.Errorf("fast_https_bin = %v", cfg["fast_https_bin"])
	}
}

func TestPreremoveStopsAgent(t *testing.T) {
	code := stripComments(readPkgFile(t, "build/packaging/linux/preremove.sh"))
	agent := strings.Index(code, "systemctl stop fast-https-agent")
	mon := strings.Index(code, "systemctl stop fast-https-monitor")
	if agent < 0 || mon < 0 || agent > mon {
		t.Fatalf("agent stop %d, monitor stop %d", agent, mon)
	}
	if !strings.Contains(code, "/usr/bin/fast-https-agent") {
		t.Fatal("preremove.sh 未把 Agent 二进制算进残留进程")
	}
}

func TestPostremovePurgesAgentConfig(t *testing.T) {
	code := stripComments(readPkgFile(t, "build/packaging/linux/postremove.sh"))
	caseIdx := strings.Index(code, `case "$1" in`)
	idx := strings.Index(code, "/etc/fast-https/agent.json")
	if caseIdx < 0 || idx < caseIdx {
		t.Fatal("purge 分支应删除 /etc/fast-https/agent.json")
	}
}

func TestSpecInstallsAgent(t *testing.T) {
	spec := readPkgFile(t, "build/spec")
	for _, want := range []string{
		"go build -o fast-https-agent",
		"install -m 0755 fast-https-agent",
		"build/packaging/agent.json",
		"%{_bindir}/fast-https-agent",
		"%config(noreplace) %{_sysconfdir}/%{name}/agent.json",
	} {
		if !strings.Contains(spec, want) {
			t.Errorf("build/spec 缺少 %q", want)
		}
	}
}

func TestNSISStopsAgent(t *testing.T) {
	nsi := readPkgFile(t, "build/installer/windows/fast-https.nsi")
	if !strings.Contains(nsi, `!define AGENT_EXE "fast-https-agent.exe"`) {
		t.Fatal("未定义 AGENT_EXE")
	}
	if strings.Count(nsi, "taskkill /F /IM ${AGENT_EXE}") != 2 {
		t.Fatal("安装和卸载都应结束 Agent 进程")
	}
}
