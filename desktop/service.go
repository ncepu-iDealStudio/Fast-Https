package main

import (
	"fast-https-desktop/internal/configops"
	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/siteops"
	"fast-https-desktop/internal/view"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fast-https-desktop/internal/cli"

	"golang.org/x/sys/windows/registry"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DesktopService 是前端能调用的 Go 方法集合，由 wails3 generate bindings
// 生成 frontend/bindings 下的 JS 客户端。数据都是进程外读取，操作转交主程序
// CLI 或 Wails 窗口 API，桌面端不接管 fast-https 服务进程。

// mainWindow 由 main.go 创建窗口后设置。用包级变量而不是 Service 字段，
// 是为了避免 bindings 生成器去分析 *WebviewWindow 这种不可序列化类型。
var mainWindow *application.WebviewWindow

func setMainWindow(w *application.WebviewWindow) { mainWindow = w }

type DesktopService struct {
	baseDir string
}

func NewDesktopService() *DesktopService {
	return &DesktopService{baseDir: view.ExeDir()}
}

// ------------------------------------------------------------ 自绘标题栏窗口按钮
// frameless 窗口没有系统标题栏按钮，全部转交 Wails 的窗口 API。

func (s *DesktopService) MinimiseWindow() {
	if mainWindow != nil {
		mainWindow.Minimise()
	}
}

func (s *DesktopService) ToggleMaximiseWindow() {
	if mainWindow != nil {
		mainWindow.ToggleMaximise()
	}
}

// HideWindow 是「隐藏到后台托盘」：隐藏窗口并保留托盘图标，
// 与 WindowClosing hook 的默认行为一致。
func (s *DesktopService) HideWindow() {
	if mainWindow != nil {
		mainWindow.Hide()
	}
}

// QuitApp 真正结束桌面端进程。关闭弹窗里的「直接退出」走这里；
// 置位 appQuitting 后 WindowClosing hook 才不会把关闭再拦回隐藏。
// 注意：这只结束桌面端，fast-https 服务由主程序独立管理，不受影响。
func (s *DesktopService) QuitApp() {
	appQuitting.Store(true)
	// 先停服务再退出：桌面端拉起服务后，关掉窗口不应该把端口一直占着。
	// 服务若不是本端启动（例如用 CLI 起的），stop 找不到 pid，属于空操作。
	_, _ = view.RunServerCommand(s.baseDir, "stop")
	if appRef != nil {
		appRef.Quit()
	}
}

// ------------------------------------------------------------ 账户菜单：帮助文档

// OpenHelp 打开随发行包一起分发的中文使用文档。
// 安装布局下 docs/ 与 exe 同级；开发时 exe 在 desktop/bin/，回退到仓库根。
func (s *DesktopService) OpenHelp() (string, error) {
	candidates := []string{
		filepath.Join(s.baseDir, "docs", "usage.html"),
		filepath.Join(s.baseDir, "..", "..", "docs", "usage.html"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return view.OpenPath(p)
		}
	}
	return "", fmt.Errorf("找不到帮助文档 docs/usage.html：已尝试 %s（发行包请把桌面端与主程序放在同一目录）", candidates[0])
}

// ------------------------------------------------------------ 设置页：主配置表单

func (s *DesktopService) ReadConfigForm() (model.ConfigForm, error) {
	return configops.ReadConfigForm(s.baseDir)
}

func (s *DesktopService) SaveConfigForm(values map[string]string) (string, error) {
	return configops.SaveConfigForm(s.baseDir, values)
}

// ------------------------------------------------------------ 站点页

func (s *DesktopService) AddSite(in model.SiteInput) (string, error) {
	return siteops.AddSite(s.baseDir, in)
}

func (s *DesktopService) ReadSiteDetail(id string) (model.SiteDetail, error) {
	return siteops.ReadSiteDetail(s.baseDir, id)
}

func (s *DesktopService) SaveSiteDetail(in model.SiteInput) (string, error) {
	return siteops.SaveSiteDetail(s.baseDir, in)
}

func (s *DesktopService) DeleteSite(id string) (string, error) {
	return siteops.DeleteSite(s.baseDir, id)
}

// SetSiteEnabled 转交 site enable/disable 的配置事务。
// 桌面端不改文件名：事务会处理校验、恢复点与 reload，和命令行语义一致。
func (s *DesktopService) SetSiteEnabled(id string, enabled bool) (string, error) {
	return siteops.SetSiteEnabled(s.baseDir, id, enabled)
}

// ------------------------------------------------------------ 本机状态与操作

func (s *DesktopService) GetOverview() (model.Overview, error) {
	return view.CollectOverview(s.baseDir), nil
}

func (s *DesktopService) ListSites() ([]model.Site, error) {
	return view.ListSites(s.baseDir)
}

func (s *DesktopService) ListCerts() ([]model.Cert, error) {
	return view.ListCerts(s.baseDir)
}

func (s *DesktopService) TailLog(name string, lines int) ([]model.LogEntry, error) {
	return view.TailLog(s.baseDir, name, lines)
}

func (s *DesktopService) Paths() (model.RuntimePaths, error) {
	return view.ResolvePaths(s.baseDir), nil
}

func (s *DesktopService) StartServer() (string, error) {
	return view.StartServer(s.baseDir)
}

func (s *DesktopService) StopServer() (string, error) {
	return view.StopServer(s.baseDir)
}

// ReloadServer 转交 service reload（受控重载：校验、operation 记录、
// reload 后验证）。服务没在跑时给出可读提示而不是报错吓人。
// Windows 上热重载信号（CTRL_BREAK）到不了别的控制台，service reload 基本
// 都会失败；此时自动改走「停止 + 启动」的重启，让新配置真正生效。
func (s *DesktopService) ReloadServer() (string, error) {
	data, _, err := cli.RunJSON[any](s.baseDir, "service", "reload")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not running") {
			return "服务未运行，无需重载；配置会在下次启动时生效", nil
		}
		if strings.Contains(msg, "not supported by windows") {
			if _, stopErr := view.StopServer(s.baseDir); stopErr == nil {
				if _, startErr := view.StartServer(s.baseDir); startErr == nil {
					return "Windows 上没有跨窗口的热重载信号，已自动重启服务，新配置已生效", nil
				}
			}
			return "", fmt.Errorf("重载失败，重启服务也未成功：%s", msg)
		}
		return "", err
	}
	m := cli.AsMap(data)
	msg := "重载完成"
	if v := cli.AsMap(m["verification"]); cli.Bool(v, "reload_confirmed") {
		msg = "重载完成并已验证"
	}
	if id := cli.Str(m, "operation_id"); id != "" {
		msg += "（操作 " + id + "）"
	}
	return msg, nil
}

func (s *DesktopService) ServerStatus() (string, error) {
	return view.RunServerCommand(s.baseDir, "status")
}

func (s *DesktopService) OpenPath(path string) (string, error) {
	return view.OpenPath(path)
}

// ImportLocalCA / QueryLogs 的实现在 view 包里，这里只做 Service 转发，
// 保持 Wails bindings 的方法集合不变。
func (s *DesktopService) ImportLocalCA() (string, error) {
	return view.ImportLocalCA(s.baseDir)
}

func (s *DesktopService) QueryLogs(q view.LogQuery) (view.LogPage, error) {
	return view.QueryLogs(s.baseDir, q)
}

func (s *DesktopService) CertTrustGuide() (model.CertTrustGuide, error) {
	return view.LocalCATrustGuide(s.baseDir)
}

// PickPath 调系统原生选择框，返回相对 config 目录的路径。
// kind 为 "dir" 时选目录，否则选文件；用户取消返回空串。
func (s *DesktopService) PickPath(kind string) (string, error) {
	if appRef == nil {
		return "", fmt.Errorf("应用尚未就绪")
	}
	etcDir := filepath.Dir(view.ResolvePaths(s.baseDir).Config)
	d := appRef.Dialog.OpenFile().SetDirectory(etcDir)
	if mainWindow != nil {
		d.AttachToWindow(mainWindow)
	}
	if kind == "dir" {
		d.SetTitle("选择站点根目录").CanChooseDirectories(true).CanChooseFiles(false)
	} else {
		d.SetTitle("选择证书文件").CanChooseDirectories(false).CanChooseFiles(true).
			AddFilter("PEM 证书", "*.pem;*.crt;*.cer").
			AddFilter("所有文件", "*.*")
	}
	p, err := d.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if p == "" {
		return "", nil
	}
	rel, relErr := filepath.Rel(etcDir, p)
	if relErr != nil {
		return p, nil
	}
	return rel, nil
}

// ------------------------------------------------------------ 开机自启动
// 写 HKCU\...\Run（当前用户级，无需管理员）：值为桌面端自身路径，
// 附 --tray 参数让开机只进托盘、不弹主窗口。注册表是自启的事实来源：
// GetAutoStart 读它画勾，SetAutoStart 增删。

const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const autostartValueName = "Fast-Https Desktop"

// autostartCommand 开机启动的完整命令行：exe + --tray（托盘常驻模式）。
func autostartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + exe + `" --tray`, nil
}

// GetAutoStart 报告是否已配置开机自启。
func (s *DesktopService) GetAutoStart() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		return false, nil // 键不存在＝没配置
	}
	defer k.Close()
	v, _, err := k.GetStringValue(autostartValueName)
	if err != nil {
		return false, nil
	}
	want, _ := autostartCommand()
	return strings.Contains(v, filepath.Base(strings.Trim(want, `"`))), nil
}

// SetAutoStart 设置或取消开机自启；返回当前状态与可读结果。
func (s *DesktopService) SetAutoStart(enabled bool) (bool, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		return false, fmt.Errorf("打开注册表失败：%w", err)
	}
	defer k.Close()
	if enabled {
		cmd, err := autostartCommand()
		if err != nil {
			return false, fmt.Errorf("取程序路径失败：%w", err)
		}
		if err := k.SetStringValue(autostartValueName, cmd); err != nil {
			return false, fmt.Errorf("写入注册表失败：%w", err)
		}
	} else {
		// 值不存在时 DeleteValue 返回错误，删除目标本就是「没有」，忽略
		_ = k.DeleteValue(autostartValueName)
	}
	return s.GetAutoStart()
}
