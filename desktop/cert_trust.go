package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CertTrustGuide 是「信任本地 CA」弹窗的数据。桌面端不擅自改系统信任库：
// 默认只把路径、风险、步骤和命令展示给用户，自动导入必须再确认一次。
type CertTrustGuide struct {
	Platform      string   `json:"platform"`
	PlatformLabel string   `json:"platformLabel"`
	CertPath      string   `json:"certPath"`
	DirPath       string   `json:"dirPath"`
	KeyPath       string   `json:"keyPath"`
	Exists        bool     `json:"exists"`
	Command       string   `json:"command"`
	Steps         []string `json:"steps"`
	Risk          string   `json:"risk"`
}

// localCAPath 取本地根 CA 证书：路径完全由主程序 `config path --json` 的
// root_crt 提供（归档 / FHS 布局由主程序按编译期基址算好），桌面端不拼
// 数据目录、也不猜安装布局。主程序不可用或旧版本没有该字段时，才回退到
// 归档布局的相对推导。
func localCAPath(dir string) string {
	p := resolvePaths(dir)
	if p.RootCRT != "" {
		return p.RootCRT
	}
	return filepath.Join(filepath.Dir(filepath.Dir(p.Config)), "httpdoc", "root", "root.crt")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func trustGuideFor(goos, path string) CertTrustGuide {
	labels := map[string]string{"windows": "Windows", "linux": "Linux", "darwin": "macOS"}
	label := labels[goos]
	if label == "" {
		label = goos
	}
	g := CertTrustGuide{
		Platform:      goos,
		PlatformLabel: label,
		CertPath:      path,
		DirPath:       filepath.Dir(path),
		// 桌面端在 Windows 与 Linux 图形环境下都会打开这个弹窗，
		// 风险说明用平台中性措辞，不写死某个平台的目录写法。
		Risk: "把这个根 CA 加入系统信任库后，本机所有 HTTPS 站点都会信任由它签发的证书。CA 私钥与证书都由服务自动生成并保存在本机配置目录，仅限本机开发使用；不要在生产环境或他人机器上导入。",
	}

	// 只有命令与「确认方式」按平台区分；通用步骤保持平台中性。
	var confirmStep string
	switch goos {
	case "windows":
		g.Command = fmt.Sprintf(`certutil -addstore -f ROOT "%s"`, path)
		confirmStep = "确认 certutil 报告该证书已加入 ROOT 存储"
	case "linux":
		g.Command = fmt.Sprintf(`sudo cp %s /usr/local/share/ca-certificates/fast-https-local-ca.crt && sudo update-ca-certificates`, shellQuote(path))
		confirmStep = `确认 update-ca-certificates 输出已添加 1 个证书；Chrome / Chromium 若仍提示，再导入 NSS 库：certutil -A -d sql:$HOME/.pki/nssdb -n "fast-https local CA" -t "C,," -i <证书路径>`
	case "darwin":
		g.Command = fmt.Sprintf(`sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s`, shellQuote(path))
		confirmStep = "在「钥匙串访问」里确认该证书被标记为始终信任"
	default:
		g.Command = ""
		confirmStep = "当前平台没有自动导入命令，请手动把证书加入系统的受信任根证书颁发机构"
	}
	g.Steps = []string{
		"用管理员权限执行上方命令（或点「尝试自动导入」让桌面端请求提权）",
		confirmStep,
		"完全退出并重启浏览器，再访问 https://localhost",
	}
	return g
}

// CertTrustGuide 返回当前平台的引导信息；证书不存在也照常返回，
// 由弹窗提示先启动服务生成，而不是直接报错。
func (s *DesktopService) CertTrustGuide() (CertTrustGuide, error) {
	paths := resolvePaths(s.baseDir)
	p := localCAPath(s.baseDir)
	g := trustGuideFor(runtime.GOOS, p)
	// CA 私钥目录与 root.crt 一样由主程序按布局给出，不在文案里写死。
	g.KeyPath = filepath.Join(paths.Cert, "root.key")
	_, err := os.Stat(p)
	g.Exists = err == nil
	return g, nil
}

// ImportLocalCA 在用户二次确认后发起提权导入。Windows 走 UAC，
// Linux 走 pkexec；失败时把系统输出带回去，让弹窗提示改用手动命令。
func (s *DesktopService) ImportLocalCA() (string, error) {
	p := localCAPath(s.baseDir)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("找不到本地根 CA 证书 %s：请先启动一次服务让它生成", p)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// Start-Process -Verb RunAs 触发 UAC；路径里的单引号按 PowerShell 规则翻倍。
		psQuote := strings.ReplaceAll(p, "'", "''")
		ps := fmt.Sprintf(`Start-Process -FilePath certutil -ArgumentList '-addstore','-f','ROOT','%s' -Verb RunAs -Wait`, psQuote)
		cmd = exec.Command("powershell", "-NoProfile", "-Command", ps)
	case "linux":
		script := fmt.Sprintf(`cp %s /usr/local/share/ca-certificates/fast-https-local-ca.crt && update-ca-certificates`, shellQuote(p))
		cmd = exec.Command("pkexec", "sh", "-c", script)
	default:
		return "", fmt.Errorf("当前平台不支持自动导入，请复制弹窗里的命令手动执行")
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("导入失败（可能取消了提权，或系统缺少提权工具）：%v\n%s",
			err, strings.TrimSpace(string(out)))
	}
	msg := "已向系统发起导入请求。请在提权弹窗中确认，然后重启浏览器再访问 https://localhost。"
	if text := strings.TrimSpace(string(out)); text != "" {
		msg += "\n" + text
	}
	return msg, nil
}
