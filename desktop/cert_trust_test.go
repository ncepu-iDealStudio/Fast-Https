package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// localCAPath 在归档布局下应落到站点根 httpdoc/root/root.crt。
func TestLocalCAPathFindsExistingRootCA(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "fast-https.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "httpdoc", "root", "root.crt")
	if err := os.MkdirAll(filepath.Dir(want), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("PEM"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := localCAPath(dir); got != want {
		t.Fatalf("localCAPath = %q, want %q", got, want)
	}
}

func TestLocalCAPathFallsBackWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "fast-https.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	got := localCAPath(dir)
	if !strings.HasSuffix(got, filepath.Join("httpdoc", "root", "root.crt")) {
		t.Fatalf("找不到证书时应返回归档布局的候选路径：%q", got)
	}
}

func TestTrustGuidePerPlatform(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		g := trustGuideFor(goos, "/tmp/root.crt")
		if g.Platform != goos || g.CertPath != "/tmp/root.crt" {
			t.Fatalf("%s: %+v", goos, g)
		}
		if g.Risk == "" || len(g.Steps) == 0 || g.Command == "" {
			t.Fatalf("%s 缺少风险/步骤/命令：%+v", goos, g)
		}
		if !strings.Contains(g.Command, "/tmp/root.crt") {
			t.Fatalf("%s 命令里没有证书路径：%q", goos, g.Command)
		}
	}
}

func TestTrustGuideWindowsQuotesPathWithSpaces(t *testing.T) {
	p := `C:\Program Files\Fast-Https\httpdoc\root\root.crt`
	g := trustGuideFor("windows", p)
	if !strings.Contains(g.Command, "certutil") || !strings.Contains(g.Command, p) {
		t.Fatalf("Windows 命令应带 certutil 与带空格的路径：%q", g.Command)
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got := shellQuote("/a b/c.crt"); got != "'/a b/c.crt'" {
		t.Fatalf("shellQuote = %q", got)
	}
	if got := shellQuote("/a'b/c"); !strings.Contains(got, `'\''`) {
		t.Fatalf("单引号未转义：%q", got)
	}
}

// 桌面端在 Windows 与 Linux 图形环境都会用这个弹窗；通用文案不能写死
// 某个平台的目录写法或工具名，命令才按平台区分。
func TestTrustGuideIsPlatformNeutral(t *testing.T) {
	win := trustGuideFor("windows", "/tmp/root.crt")
	linux := trustGuideFor("linux", "/tmp/root.crt")
	mac := trustGuideFor("darwin", "/tmp/root.crt")

	if win.PlatformLabel != "Windows" || linux.PlatformLabel != "Linux" || mac.PlatformLabel != "macOS" {
		t.Fatalf("平台标签不对：%s / %s / %s", win.PlatformLabel, linux.PlatformLabel, mac.PlatformLabel)
	}
	for _, g := range []CertTrustGuide{win, linux, mac} {
		if strings.Contains(g.Risk, "config/cert/root.key") {
			t.Fatalf("风险说明写死了归档路径：%q", g.Risk)
		}
		if strings.Contains(g.Steps[0], "PowerShell") || strings.Contains(g.Steps[0], "pkexec") {
			t.Fatalf("通用步骤不应带平台专属工具：%q", g.Steps[0])
		}
	}
	if strings.Contains(strings.Join(linux.Steps, " "), "UAC") || strings.Contains(linux.Command, "certutil -addstore") {
		t.Fatal("Linux 引导里混入了 Windows 做法")
	}
	if strings.Contains(win.Command, "update-ca-certificates") || strings.Contains(win.Command, "pkexec") {
		t.Fatal("Windows 引导里混入了 Linux 做法")
	}
}

func TestCertTrustGuideReportsMissingCert(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "fast-https.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	s := &DesktopService{baseDir: dir}
	g, err := s.CertTrustGuide()
	if err != nil {
		t.Fatal(err)
	}
	if g.Exists {
		t.Fatal("证书不存在时 Exists 应为 false")
	}
	if g.CertPath == "" || len(g.Steps) == 0 {
		t.Fatalf("证书缺失也要给出路径与步骤：%+v", g)
	}
	if !strings.HasSuffix(g.KeyPath, "root.key") {
		t.Fatalf("CA 私钥路径应由主程序给出的证书目录推导：%q", g.KeyPath)
	}
}
