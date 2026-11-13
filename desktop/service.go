package main

import (
	"fast-https-desktop/internal/configops"
	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/siteops"
	"fast-https-desktop/internal/view"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	return configops.ReadConfigForm(s.baseDir)
}

func (s *DesktopService) SaveConfigForm(values map[string]string) (string, error) {
	return configops.SaveConfigForm(s.baseDir, values)
}

// ------------------------------------------------------------ 站点页

func (s *DesktopService) ReadSiteDetail(id string) (model.SiteDetail, error) {
	return siteops.ReadSiteDetail(s.baseDir, id)
}

// invalidateAfterSiteWrite 站点写操作后清掉 site list 缓存。
// 不清的话 refreshDetails 会读到 5 秒内的旧缓存，把刚更新的状态回滚。
func (s *DesktopService) invalidateAfterSiteWrite() {
	view.InvalidateSiteCache()
}

func (s *DesktopService) AddSite(in model.SiteInput) (string, error) {
	msg, err := siteops.AddSite(s.baseDir, in)
	if err == nil {
		s.invalidateAfterSiteWrite()
	}
	return msg, err
}

func (s *DesktopService) SaveSiteDetail(in model.SiteInput) (string, error) {
	msg, err := siteops.SaveSiteDetail(s.baseDir, in)
	if err == nil {
		s.invalidateAfterSiteWrite()
	}
	return msg, err
}

func (s *DesktopService) DeleteSite(id string) (string, error) {
	msg, err := siteops.DeleteSite(s.baseDir, id)
	if err == nil {
		s.invalidateAfterSiteWrite()
	}
	return msg, err
}

// SetSiteEnabled 转交 site enable/disable 的配置事务。
// 桌面端不改文件名：事务会处理校验、恢复点与 reload，和命令行语义一致。
// 成功后立刻让站点列表缓存失效，保证紧接着的刷新拿到的是新状态。
func (s *DesktopService) SetSiteEnabled(id string, enabled bool) (string, error) {
	msg, err := siteops.SetSiteEnabled(s.baseDir, id, enabled)
	if err == nil {
		s.invalidateAfterSiteWrite()
	}
	return msg, err
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

// StartServer 启动服务，并顺带把独立的 Monitor 进程拉起来。
// Monitor 只负责采集本机动态，起不来不该阻止开站，所以它失败只忽略。
func (s *DesktopService) StartServer() (string, error) {
	return view.StartServerWithMonitor(s.baseDir)
}

// StopServer 停服务并一并停掉 Monitor，避免残留进程继续写事件。
func (s *DesktopService) StopServer() (string, error) {
	return view.StopServerWithMonitor(s.baseDir)
}

// ReloadServer 让新配置生效。
//
// Windows 上不存在可用的热重载：GenerateConsoleCtrlEvent 只能发给整个
// 控制台进程组，定向发给服务会连带打断发起方（桌面端自己）。所以
// service reload 在 Windows 必然失败，每次都先跑一趟再回退只会让用户
// 看到一次无谓的失败提示、并多花一个超时周期。
//
// 这里改为：Windows 直接走「停止 + 启动」，其它平台仍用 service reload
// （受控重载：校验、operation 记录、reload 后验证）。
func (s *DesktopService) ReloadServer() (string, error) {
	// 服务没在跑就不用重载，给可读提示而不是报错
	if !view.ServerRunning(s.baseDir) {
		return "服务未运行，无需重载；配置会在下次启动时生效", nil
	}

	if runtime.GOOS == "windows" {
		return s.restartForReload()
	}

	data, _, err := cli.RunJSON[any](s.baseDir, "service", "reload")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not running") {
			return "服务未运行，无需重载；配置会在下次启动时生效", nil
		}
		// 其它平台也可能因为拿不到信号而失败，同样回退到重启
		if strings.Contains(msg, "not supported") {
			return s.restartForReload()
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

// restartForReload 用「停止 + 启动」应用新配置，等新进程真正就绪再返回。
// 这是 Windows 上唯一可靠的配置生效方式。
func (s *DesktopService) restartForReload() (string, error) {
	if _, stopErr := view.StopServerWithMonitor(s.baseDir); stopErr != nil {
		return "", fmt.Errorf("重载失败：停止服务时出错（%v）", stopErr)
	}
	if _, startErr := view.StartServer(s.baseDir); startErr != nil {
		return "", fmt.Errorf("重载失败：服务已停止但启动失败（%v），请手动启动", startErr)
	}
	// 等端口真正可连，避免界面刷新时读到「启动中」的中间态
	if view.WaitServerRunning(s.baseDir, 8*time.Second) {
		// 服务已就绪，把 Monitor 重新拉起来
		if view.MonitorInstalled(s.baseDir) {
			if err := view.StartMonitor(s.baseDir); err != nil {
				return "已重启服务，新配置已生效；Monitor 未能启动", nil
			}
			view.WaitMonitorRunning(s.baseDir, 6*time.Second)
		}
		return "已重启服务，新配置已生效", nil
	}
	return "服务正在启动，配置将在就绪后生效", nil
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
