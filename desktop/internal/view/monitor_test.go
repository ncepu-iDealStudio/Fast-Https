package view

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// monitorExeName 返回当前平台上 Monitor 可执行文件的名字。
func monitorExeName() string {
	if runtime.GOOS == "windows" {
		return "fast-https-monitor.exe"
	}
	return "fast-https-monitor"
}

// touchMonitorBinary 在 dir 下放一个占位的 Monitor 可执行文件。
// 只测路径判定，不需要真能运行。
func touchMonitorBinary(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, monitorExeName())
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// 没放 exe 时应当判定为未安装，而不是盲目尝试启动。
func TestMonitorInstalledFalseWithoutBinary(t *testing.T) {
	dir := t.TempDir()
	if MonitorInstalled(dir) {
		t.Fatalf("目录里没有 Monitor 却报告已安装")
	}
}

func TestMonitorInstalledTrueWithBinary(t *testing.T) {
	dir := t.TempDir()
	touchMonitorBinary(t, dir)
	if !MonitorInstalled(dir) {
		t.Fatalf("放了 Monitor 却报告未安装")
	}
}

// 缺失 exe 时 StartMonitor 必须给出可读错误，不能静默成功。
func TestStartMonitorMissingBinary(t *testing.T) {
	dir := t.TempDir()
	if err := StartMonitor(dir); err == nil {
		t.Fatalf("没有 Monitor 时启动应当报错")
	}
}

// 状态文件不存在时，monitorStatusFresh 必须为 false。
// 这是「进程在跑 ≠ 面板有数据」的判据：只靠进程会刷出中间的空白态。
func TestMonitorStatusFreshWithoutFile(t *testing.T) {
	dir := writeSiteWorkspace(t)
	if monitorStatusFresh(dir) {
		t.Fatalf("没有状态文件却报告新鲜")
	}
}

// 状态文件存在但为空时同样不算新鲜（Monitor 写文件是原子的，
// 空文件只可能是写了一半或被人清空）。
func TestMonitorStatusFreshEmptyFile(t *testing.T) {
	dir := writeSiteWorkspace(t)
	mdir := filepath.Join(dir, "logs", "monitor")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mdir, "status.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if monitorStatusFresh(dir) {
		t.Fatalf("空状态文件不该算新鲜")
	}
}

func TestMonitorStatusFreshWithContent(t *testing.T) {
	dir := writeSiteWorkspace(t)
	mdir := filepath.Join(dir, "logs", "monitor")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":"1","status":"running"}`
	if err := os.WriteFile(filepath.Join(mdir, "status.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !monitorStatusFresh(dir) {
		t.Fatalf("有状态文件却报告不新鲜")
	}
}

// 用户把日志目录配到别处时，要按 log_root 找状态文件，而不是死认 ./logs。
func TestMonitorStatusFreshHonoursLogRoot(t *testing.T) {
	dir := t.TempDir()
	etc := filepath.Join(dir, "config")
	if err := os.MkdirAll(etc, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dir, "custom-logs")
	body := `{"log_root":"` + filepath.ToSlash(custom) + `"}`
	if err := os.WriteFile(filepath.Join(etc, "fast-https.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	mdir := filepath.Join(custom, "monitor")
	if err := os.MkdirAll(mdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mdir, "status.json"), []byte(`{"status":"running"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !monitorStatusFresh(dir) {
		t.Fatalf("改了 log_root 后没找到状态文件")
	}
}

// 开发态 Monitor 常在 desktop/bin 或仓库根，monitorBinaryFor 要能找到它，
// 否则 MonitorInstalled 说「已装」、StartMonitor 却只会去 dir 下找。
func TestMonitorBinaryForDevLayouts(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "desktop", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(binDir, monitorExeName())
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !MonitorInstalled(dir) {
		t.Fatalf("desktop/bin 下的 Monitor 没被探测到")
	}
	if got := monitorBinaryFor(dir); got != p {
		t.Fatalf("应解析到 %s，得到 %s", p, got)
	}
}

// 三个候选路径都没放 exe 时，monitorBinaryFor 退回默认位置，
// StartMonitor 据此报「未找到」而不是去 PATH 里碰运气。
func TestMonitorBinaryForFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, monitorExeName())
	if got := monitorBinaryFor(dir); got != want {
		t.Fatalf("应退回 %s，得到 %s", want, got)
	}
}

// MonitorBinary 必须与主程序同目录，安装包里两者是平级的。
func TestMonitorBinarySameDirAsServer(t *testing.T) {
	dir := t.TempDir()
	got := MonitorBinary(dir)
	if filepath.Dir(got) != dir {
		t.Fatalf("Monitor 路径不在 dir 下：%s", got)
	}
	if filepath.Dir(got) != filepath.Dir(ServerBinary(dir)) {
		t.Fatalf("Monitor 与主程序不在同一目录")
	}
	if filepath.Base(got) != monitorExeName() {
		t.Fatalf("文件名不对：%s", filepath.Base(got))
	}
}
