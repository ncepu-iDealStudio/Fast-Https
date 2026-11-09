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
	return configops.ReadConfigForm(view.ResolvePaths(s.baseDir).Config)
}

func (s *DesktopService) SaveConfigForm(values map[string]string) (string, error) {
	return configops.SaveConfigForm(view.ResolvePaths(s.baseDir).Config, values)
}

// ------------------------------------------------------------ 站点页：添加站点

func (s *DesktopService) AddSite(in model.SiteInput) (string, error) {
	return siteops.AddSiteFile(s.baseDir, in)
}

func (s *DesktopService) ReadSiteDetail(source string) (model.SiteDetail, error) {
	return siteops.ReadSiteDetail(s.baseDir, source)
}

func (s *DesktopService) SaveSiteDetail(in model.SiteInput) (string, error) {
	return siteops.SaveSiteDetail(s.baseDir, in)
}

func (s *DesktopService) DeleteSite(source string) (string, error) {
	return siteops.DeleteSiteFile(s.baseDir, source)
}

// SetSiteEnabled 转交 site enable/disable 的配置事务。
// 桌面端不改文件名：事务会处理校验、恢复点与 reload，和命令行语义一致。
func (s *DesktopService) SetSiteEnabled(source string, enabled bool) (string, error) {
	name := filepath.Base(strings.TrimSpace(source))
	name = strings.TrimSuffix(name, ".disabled")
	name = strings.TrimSuffix(name, ".json")
	if name == "" {
		return "", fmt.Errorf("缺少站点来源")
	}
	action, word := "disable", "停用"
	if enabled {
		action, word = "enable", "启用"
	}
	if _, _, err := cli.RunJSON[any](s.baseDir, "site", action, name); err != nil {
		return "", err
	}
	return fmt.Sprintf("已%s站点 %s（配置事务完成）", word, name), nil
}

// ------------------------------------------------------------ 本机状态与操作

func (s *DesktopService) GetOverview() (model.Overview, error) {
	return view.CollectOverview(s.baseDir), nil
}

func (s *DesktopService) ListSites() ([]model.Site, error) {
	sites, _, err := view.ReadSites(s.baseDir)
	return sites, err
}

func (s *DesktopService) ListCerts() ([]model.Cert, error) {
	return view.ReadCerts(s.baseDir)
}

func (s *DesktopService) TailLog(name string, lines int) ([]string, error) {
	return view.TailLog(view.ResolvePaths(s.baseDir).Log, name, lines)
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

func (s *DesktopService) ReloadServer() (string, error) {
	return view.RunServerCommand(s.baseDir, "reload")
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
	return view.QueryLogs(view.ResolvePaths(s.baseDir).Log, q)
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
