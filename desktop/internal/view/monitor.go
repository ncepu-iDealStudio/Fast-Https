// Monitor 是独立的 fast-https-monitor.exe 进程：它周期读本机 /runtime，
// 把快照与事件写进 logs/monitor/。GUI 的 Monitor 面板只是读那个状态文件，
// 所以进程不跑起来，面板永远显示「未安装」。
//
// 这里负责在桌面端启停服务时顺带把 Monitor 也带起来/停掉。为什么不交给
// 主程序自己拉：Monitor 的设计边界是「不被 Server 启动」，Server 崩了它
// 还要能记录，所以生命周期由桌面端这个第三方来管最合适。

package view

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// MonitorBinary 返回与本实例同目录的 Monitor 可执行文件路径。
// 安装包里它和主程序、桌面端平级，取自 dir 而不是 exe 所在目录，
// 与 ServerBinary 保持同一套布局假设。
func MonitorBinary(dir string) string {
	name := "fast-https-monitor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// MonitorInstalled 报告 Monitor 可执行文件是否就位。
// 打包漏了、或用户手工删了，都靠它区分「没装」和「装了没启动」。
//
// 只认 monitorCandidates 里那几个明确位置：不用 exec.LookPath 兜底，否则
// PATH 里任何一个同名文件都会让「未安装」被误判成「已安装」，面板就会
// 一直转圈而不是给出明确结论。开发态布局也由这几个候选覆盖。
func MonitorInstalled(dir string) bool {
	for _, p := range monitorCandidates(dir) {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// monitorCandidates 给出 Monitor 可执行文件可能的位置，按优先级排序。
//
//   - dir：安装包里 exe 与主程序平级
//   - dir/desktop/bin：开发态桌面端从 desktop/bin 运行，Monitor 由
//     go build -o bin/ 产出到同一处
//   - 仓库根：开发态 dir 是 desktop/，Monitor 在仓库根
func monitorCandidates(dir string) []string {
	name := "fast-https-monitor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return []string{
		filepath.Join(abs, name),
		filepath.Join(abs, "desktop", "bin", name),
		filepath.Join(abs, "..", "..", name),
	}
}

// monitorBinaryFor 返回实际存在的那个 Monitor 路径，找不到时退回 dir 下的默认位置。
func monitorBinaryFor(dir string) string {
	for _, p := range monitorCandidates(dir) {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return MonitorBinary(dir)
}

// StartMonitor 以分离方式拉起 Monitor，已在运行则直接返回。
//
// 与 StartServer 用同一套分离参数：桌面端关到托盘或退出时不能把
// Monitor 带走，否则面板又回到「未安装」。
func StartMonitor(dir string) error {
	if MonitorRunning(dir) {
		return nil
	}
	exe := monitorBinaryFor(dir)
	// 与 MonitorInstalled 同口径：只认三个明确位置，不用 LookPath 兜底，
	// 免得 PATH 里的同名文件让「没装」被误判成「装了」。
	found := false
	for _, p := range monitorCandidates(dir) {
		if _, err := os.Stat(p); err == nil {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("未找到 Monitor 程序 %s", filepath.Base(exe))
	}
	cmd := exec.Command(exe)
	cmd.Dir = dir
	setDetached(cmd)
	// 配置与日志目录都用相对 dir 的默认值：Monitor 自己会读
	// config/monitor.json，缺省时按内置默认跑。stdout/stderr 丢掉，
	// 否则桌面端退出后管道断开会让它报错。
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Monitor 失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// StopMonitor 停止 Monitor 进程。先礼后兵：Windows 上 Monitor 没有
// 控制台信号可发，直接按 exe 路径结束本实例的那一个，不碰别的安装。
func StopMonitor(dir string) {
	exe, err := filepath.Abs(monitorBinaryFor(dir))
	if err != nil {
		return
	}
	for _, p := range GetProcessesByName(filepath.Base(exe)) {
		if strings.EqualFold(p.Path, exe) || p.Path == "" {
			_ = KillProcess(int(p.Pid))
		}
	}
}

// MonitorRunning 报告 Monitor 进程是否在跑。
func MonitorRunning(dir string) bool {
	exe, err := filepath.Abs(monitorBinaryFor(dir))
	if err != nil {
		return false
	}
	for _, p := range GetProcessesByName(filepath.Base(exe)) {
		if strings.EqualFold(p.Path, exe) || p.Path == "" {
			return true
		}
	}
	return false
}

// WaitMonitorRunning 轮询等 Monitor 起来并写出第一份状态文件。
//
// 两层条件缺一不可：进程在跑 ≠ 面板有数据，状态文件是它第一次采集成功后
// 才写的。只等进程会让界面刷出「运行中但无数据」的中间态。
func WaitMonitorRunning(dir string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if MonitorRunning(dir) && monitorStatusFresh(dir) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return MonitorRunning(dir)
}

// monitorStatusFresh 报告 Monitor 是否已写出可读的状态文件。
//
// 这里不 import management/monview：desktop 是独立 Go 模块，拿不到主模块
// 的内部包。状态文件的路径约定很稳定（日志目录/monitor/status.json，
// 与 monview.StatusFile、monitor.StatusFile 一致），存在且非空即可认为
// Monitor 已经跑起来并采集成功过一次。
//
// 日志目录取主配置的 log_root（和 monitor status 命令读的是同一个），
// 取不到再退回 ResolvePaths 的默认值，避免用户改过 log_root 后这里判错。
func monitorStatusFresh(dir string) bool {
	for _, root := range monitorLogRoots(dir) {
		info, err := os.Stat(filepath.Join(root, "monitor", "status.json"))
		if err == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

// monitorLogRoots 给出可能的状态文件所在目录，按可信度排序。
func monitorLogRoots(dir string) []string {
	var roots []string
	if root := logRootFromConfig(dir); root != "" {
		roots = append(roots, root)
	}
	roots = append(roots, ResolvePaths(dir).Log, filepath.Join(dir, "logs"))
	return roots
}

// logRootFromConfig 读主配置的 log_root，与 logquery.ResolveLogDir 同口径。
// 只做最朴素的解析：读不到、不是 JSON、没写这个键都返回空串让调用方兜底。
func logRootFromConfig(dir string) string {
	data, err := os.ReadFile(ResolvePaths(dir).Config)
	if err != nil {
		return ""
	}
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return ""
	}
	v, _ := raw["log_root"].(string)
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !filepath.IsAbs(v) {
		if abs, err := filepath.Abs(filepath.Join(dir, v)); err == nil {
			return abs
		}
	}
	return v
}

// StartServerWithMonitor 启动服务，随后把 Monitor 也拉起来。
//
// Monitor 依赖 /runtime 接口，必须等服务就绪后再启动；启动失败不影响
// 服务本身——Monitor 只是观测工具，不该因为它没起来就让用户无法开站。
func StartServerWithMonitor(dir string) (string, error) {
	msg, err := StartServer(dir)
	if err != nil {
		return msg, err
	}
	if !MonitorInstalled(dir) {
		return msg, nil
	}
	// 服务就绪前 Monitor 第一轮采集必然失败，等一小会儿再拉
	if WaitServerRunning(dir, 8*time.Second) {
		_ = StartMonitor(dir)
	}
	return msg, nil
}

// StopServerWithMonitor 先停服务再停 Monitor。
//
// 顺序不能反：Monitor 先停会把「服务不可达」写进事件，用户卸载/重载时
// 会看到一条假告警。停服务后它自己采集失败才是真实情况，而这时我们也
// 不再需要它了。
func StopServerWithMonitor(dir string) (string, error) {
	msg, err := StopServer(dir)
	StopMonitor(dir)
	return msg, err
}
