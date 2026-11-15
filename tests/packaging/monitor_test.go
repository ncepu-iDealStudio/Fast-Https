package packaging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goreleaserMonitors 是 nfpms 段里都该出现的行数。
// 桌面端只发布 Windows（NSIS），Linux 桌面包 deb/rpm/apk 已移除，
// 所以这里只剩主程序的服务端包一个 nfpm 段。
const nfpmEntries = 1

func readPkgFile(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", rel, err)
	}
	// 开发机 core.autocrlf=true 时工作区是 CRLF，按行拼接的断言要先把行尾统一。
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

// Monitor 的单元必须能独立于 Server 运行：Server 没起时它要照常采集并写「不可达」。
// 一旦有人给它加上对 fast-https.service 的硬依赖，这个观测能力就没了。
func TestMonitorUnitIsIndependentOfServer(t *testing.T) {
	unit := readPkgFile(t, "build/packaging/systemd/fast-https-monitor.service")

	for _, want := range []string{
		"ExecStart=/usr/bin/fast-https-monitor",
		"-config /etc/fast-https/monitor.json",
		"-dir /var/log/fast-https/monitor",
		"Restart=on-failure",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("Monitor 单元缺少 %q：装完要么起不来，要么写错目录", want)
		}
	}

	for _, banned := range []string{
		"Requires=fast-https",
		"BindsTo=fast-https",
		"PartOf=fast-https",
		"After=fast-https.service",
	} {
		if strings.Contains(unit, banned) {
			t.Errorf("Monitor 单元不应包含 %q：Server 起不来时 Monitor 仍要能写「不可达」", banned)
		}
	}
}

// 反方向同样要守：Server 单元不依赖 Monitor，停掉 Monitor 不影响业务。
func TestServerUnitDoesNotDependOnMonitor(t *testing.T) {
	unit := readPkgFile(t, "build/packaging/systemd/fast-https.service")
	if strings.Contains(unit, "fast-https-monitor") {
		t.Error("fast-https.service 不应引用 Monitor：监控挂了不能连带影响数据面")
	}
}

// 打包条目最容易漏：加了二进制却没进包，或者进了主程序包却忘了三个桌面包。
func TestGoreleaserShipsMonitor(t *testing.T) {
	yaml := readPkgFile(t, "build/goreleaser.yaml")

	if n := strings.Count(yaml, "main: ./monitor"); n < 3 {
		t.Errorf("goreleaser 的 builds 里只有 %d 处 main: ./monitor，期望 3 处（unix/windows/发行版包）", n)
	}
	if n := strings.Count(yaml, "binary: fast-https-monitor"); n != strings.Count(yaml, "main: ./monitor") {
		t.Errorf("binary 与 main 数量不一致：有构建没指定二进制名")
	}
	// 四个 nfpm 段都要带上发行版包专用的 monitor 构建
	if n := strings.Count(yaml, "- monitor-linux-pkg"); n != nfpmEntries {
		t.Errorf("nfpms 里有 %d 处 monitor-linux-pkg，期望 %d", n, nfpmEntries)
	}
	// 单元与配置各自要装 4 次（主程序包 + 三个桌面包）
	if n := strings.Count(yaml, "dst: /usr/lib/systemd/system/fast-https-monitor.service"); n != nfpmEntries {
		t.Errorf("goreleaser 里有 %d 处安装 Monitor 单元，期望 %d", n, nfpmEntries)
	}
	if n := strings.Count(yaml, "dst: /etc/fast-https/monitor.json"); n != nfpmEntries {
		t.Errorf("goreleaser 里有 %d 处安装 Monitor 配置，期望 %d", n, nfpmEntries)
	}
	// 配置必须是 conffile：升级不能覆盖运维调过的阈值
	conffile := "dst: /etc/fast-https/monitor.json\n        type: config|noreplace"
	if n := strings.Count(yaml, conffile); n != nfpmEntries {
		t.Errorf("Monitor 配置只有 %d 处登记为 config|noreplace，期望 %d", n, nfpmEntries)
	}
	// 归档（tar.gz / Windows zip）也要带二进制，否则解压用户拿不到 Monitor
	for _, want := range []string{"- monitor-unix", "- monitor-windows"} {
		if !strings.Contains(yaml, want) {
			t.Errorf("archives 里缺少 %q：归档里没有 Monitor 二进制", want)
		}
	}
}

// 包内配置写的是 FHS 绝对路径：Monitor 自己的默认值是相对工作目录的，
// 单元虽已用 -dir/-config 覆盖，配置里也必须是包布局，避免两处说法不一致。
func TestPackagedMonitorConfigUsesFHSPaths(t *testing.T) {
	raw := readPkgFile(t, "build/packaging/monitor.json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("monitor.json 不是合法 JSON: %v", err)
	}

	if got := cfg["dir"]; got != "/var/log/fast-https/monitor" {
		t.Errorf("dir = %v，期望 /var/log/fast-https/monitor", got)
	}
	if got := cfg["pid_file"]; got != "/run/fast-https/fast-https.pid" {
		t.Errorf("pid_file = %v，期望 /run/fast-https/fast-https.pid", got)
	}
	if got, _ := cfg["snapshot_url"].(string); !strings.Contains(got, "127.0.0.1:9088/runtime") {
		t.Errorf("snapshot_url = %v，期望指向本机只读接口", got)
	}
	// 阈值与开关显式写出来：默认值随代码变时，包里的口径要有人看一眼。
	for _, key := range []string{"interval", "timeout", "cooldown", "five_xx_ratio", "five_xx_min_requests", "port_probe", "stuck_hint"} {
		if _, ok := cfg[key]; !ok {
			t.Errorf("monitor.json 缺少 %q：包内配置应把口径写全，而不是依赖二进制默认值", key)
		}
	}
}

// 卸载语义：先停 Monitor 再停 Server，且 purge 才删事件与配置。
func TestPreremoveStopsMonitorBeforeServer(t *testing.T) {
	code := stripComments(readPkgFile(t, "build/packaging/linux/preremove.sh"))

	mon := strings.Index(code, "systemctl stop fast-https-monitor")
	srv := strings.Index(code, "systemctl stop fast-https >")
	if mon < 0 {
		t.Fatal("preremove.sh 未停止 fast-https-monitor 单元：包删了 Monitor 还在跑")
	}
	if srv < 0 {
		t.Fatal("preremove.sh 未停止 fast-https 单元")
	}
	if mon > srv {
		t.Error("preremove.sh 应先停 Monitor 再停 Server，否则卸载会在事件里留一条假的「不可达」")
	}

	// 手工启动的 Monitor 不在单元 cgroup 里，必须能靠 /proc 扫描认出来。
	if !strings.Contains(code, "/usr/bin/fast-https-monitor") {
		t.Error("preremove.sh 未把 Monitor 二进制算进「我们的进程」，手工启动的残留杀不掉")
	}
}

func TestPostremovePurgesMonitorConfig(t *testing.T) {
	code := stripComments(readPkgFile(t, "build/packaging/linux/postremove.sh"))

	caseIdx := strings.Index(code, `case "$1" in`)
	if caseIdx < 0 {
		t.Fatal(`postremove.sh 缺少 case "$1" in`)
	}
	for _, target := range []string{"/etc/fast-https/monitor.json", "/var/log/fast-https"} {
		idx := strings.Index(code, target)
		if idx < 0 {
			t.Errorf("postremove.sh 的 purge 分支应清理 %s", target)
			continue
		}
		if idx < caseIdx {
			t.Errorf("%s 的删除语句出现在 case 之前，普通 remove 也会删掉事件与配置", target)
		}
	}
}

// Windows 安装包靠 File /r 把 zip 里的文件整体摊开，二进制会自动带上；
// 但 NSIS 只结束自己认识的那几个进程名——漏掉 Monitor 会让升级替换和卸载删除
// 因为文件被占用而失败，表现是「装完还是旧版本」「卸完目录还在」。
func TestNSISStopsMonitor(t *testing.T) {
	nsi := readPkgFile(t, "build/installer/windows/fast-https.nsi")

	if !strings.Contains(nsi, `!define MONITOR_EXE "fast-https-monitor.exe"`) {
		t.Fatal(`fast-https.nsi 未定义 MONITOR_EXE 为 "fast-https-monitor.exe"`)
	}
	kill := "taskkill /F /IM ${MONITOR_EXE}"
	uninstallIdx := strings.Index(nsi, `Section "Uninstall"`)
	if uninstallIdx < 0 {
		t.Fatal(`fast-https.nsi 缺少 Section "Uninstall"`)
	}

	before := strings.Count(nsi[:uninstallIdx], kill)
	after := strings.Count(nsi[uninstallIdx:], kill)
	if before != 1 {
		t.Errorf("安装段结束 Monitor 进程 %d 次，期望 1 次：升级时旧进程占着文件", before)
	}
	if after != 1 {
		t.Errorf("卸载段结束 Monitor 进程 %d 次，期望 1 次：残留进程会让 RMDir /r 删不干净", after)
	}
}

// OpenEuler / COPR 走 build/spec 而不是 goreleaser，二进制的构建与安装不能只改一边。
func TestSpecInstallsMonitor(t *testing.T) {
	spec := readPkgFile(t, "build/spec")

	for _, want := range []string{
		"go build -o fast-https-monitor",
		"install -m 0755 fast-https-monitor",
		"build/packaging/monitor.json",
		"%{_bindir}/fast-https-monitor",
		"%config(noreplace) %{_sysconfdir}/%{name}/monitor.json",
	} {
		if !strings.Contains(spec, want) {
			t.Errorf("build/spec 缺少 %q：spec 打出来的包会没有 Monitor", want)
		}
	}
}
