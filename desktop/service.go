package main

import (
	"fmt"
	"os"
	"path/filepath"

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
	return &DesktopService{baseDir: exeDir()}
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
	_, _ = runServerCommand(s.baseDir, "stop")
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
			return openPath(p)
		}
	}
	return "", fmt.Errorf("找不到帮助文档 docs/usage.html：已尝试 %s（发行包请把桌面端与主程序放在同一目录）", candidates[0])
}

// ------------------------------------------------------------ 设置页：主配置表单

func (s *DesktopService) ReadConfigForm() (ConfigForm, error) {
	return readConfigForm(resolvePaths(s.baseDir).Config)
}

func (s *DesktopService) SaveConfigForm(values map[string]string) (string, error) {
	return saveConfigForm(resolvePaths(s.baseDir).Config, values)
}

// ------------------------------------------------------------ 站点页：添加站点

func (s *DesktopService) AddSite(in SiteInput) (string, error) {
	return addSiteFile(s.baseDir, in)
}

func (s *DesktopService) ReadSiteDetail(source string) (SiteDetail, error) {
	return readSiteDetail(s.baseDir, source)
}

func (s *DesktopService) SaveSiteDetail(in SiteInput) (string, error) {
	return saveSiteDetail(s.baseDir, in)
}

func (s *DesktopService) DeleteSite(source string) (string, error) {
	return deleteSiteFile(s.baseDir, source)
}

// SetSiteEnabled 启用/停用 conf.d 下的站点（改文件名，需手动重载生效）。
func (s *DesktopService) SetSiteEnabled(source string, enabled bool) (string, error) {
	return setSiteEnabled(s.baseDir, source, enabled)
}

// ------------------------------------------------------------ 本机状态与操作

func (s *DesktopService) GetOverview() (Overview, error) {
	return collectOverview(s.baseDir), nil
}

func (s *DesktopService) ListSites() ([]Site, error) {
	sites, _, err := readSites(s.baseDir)
	return sites, err
}

func (s *DesktopService) ListCerts() ([]Cert, error) {
	return readCerts(s.baseDir)
}

func (s *DesktopService) TailLog(name string, lines int) ([]string, error) {
	return tailLog(resolvePaths(s.baseDir).Log, name, lines)
}

func (s *DesktopService) Paths() (RuntimePaths, error) {
	return resolvePaths(s.baseDir), nil
}

func (s *DesktopService) StartServer() (string, error) {
	return startServer(s.baseDir)
}

func (s *DesktopService) StopServer() (string, error) {
	return stopServer(s.baseDir)
}

func (s *DesktopService) ReloadServer() (string, error) {
	return runServerCommand(s.baseDir, "reload")
}

func (s *DesktopService) ServerStatus() (string, error) {
	return runServerCommand(s.baseDir, "status")
}

func (s *DesktopService) OpenPath(path string) (string, error) {
	return openPath(path)
}
