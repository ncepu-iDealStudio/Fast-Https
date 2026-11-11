package view

// 桌面端是 CLI 的图形前端：状态与数据全部来自 `fast-https <cmd> --json`
// 的 data 投影（形状见 docs/cli/cli-json-shapes.md），不再自己解析配置、
// 证书与日志文件。这里只做两件纯 UI 的事：把命令数据映射成界面视图模型，
// 以及窗口/托盘相关的本地行为（打开路径、启动/停止转交）。
//
// 唯一保留的文件读取是「候选配置树」的搭建（configops）与纯 UI 元数据
// （stat 大小、修改时间），不读配置内容。

import (
	"encoding/json"
	"errors"
	"fast-https-desktop/internal/cli"
	"fast-https-desktop/internal/model"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const desktopVersion = "1.7.0"

// RuntimePaths 与 cmd/internal/configcmd/path.go 的 RuntimePaths 一致。
type RuntimePaths = model.RuntimePaths

// Overview 是概览页一次拉取的全部内容。
type Overview = model.Overview

// Site 是站点页的一行（site list 的一个站点按监听拆行）。
type Site = model.Site

// Cert 是 cert list 的一行。
type Cert = model.Cert

type LogEntry = model.LogEntry
type LogQuery = model.LogQuery
type LogPage = model.LogPage

func ExeDir() string {
	e, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(e)
}

func ServerBinary(dir string) string {
	name := "fast-https"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// ------------------------------------------------------------ 命令结果缓存
// 概览每 2 秒轮询一次，每次都把 site list / cert list / config show 各起一个
// 进程太浪费。短 TTL 缓存把同一次刷新周期里的重复调用合并掉；缓存 key 带
// 目录与参数，测试用不同 TempDir 互不影响。

type cacheEntry struct {
	at   time.Time
	data any
	err  error
}

var (
	cacheMu  sync.Mutex
	cmdCache = map[string]cacheEntry{}
)

func cached(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	cacheMu.Lock()
	if e, ok := cmdCache[key]; ok && time.Since(e.at) < ttl {
		cacheMu.Unlock()
		return e.data, e.err
	}
	cacheMu.Unlock()
	data, err := fn()
	cacheMu.Lock()
	cmdCache[key] = cacheEntry{at: time.Now(), data: data, err: err}
	cacheMu.Unlock()
	return data, err
}

// InvalidateSiteCache 让站点列表缓存立刻失效。
//
// 站点写操作（启停/增删改）完成后必须调用：site list 有 5 秒 TTL，
// 不失效的话，紧接着的 refreshDetails 会拿到缓存里的旧数据，把界面
// 刚乐观更新的新状态「回滚」成旧状态——用户看到的现象是：点停用后
// 要等几秒才变红。写操作后直接清缓存，下一次读取必定重新拉 CLI。
func InvalidateSiteCache() {
	cacheMu.Lock()
	for k := range cmdCache {
		if strings.HasPrefix(k, "site.list:") {
			delete(cmdCache, k)
		}
	}
	cacheMu.Unlock()
}

// ------------------------------------------------------------ 运行期路径

// ResolvePaths 优先让主程序自己报路径（归档 / deb / rpm / apk 布局不同），
// 报不出来再按归档布局兜底。结果短 TTL 缓存，避免每次刷新都起进程。
func ResolvePaths(dir string) RuntimePaths {
	v, _ := cached("paths:"+dir, 3*time.Second, func() (any, error) {
		return resolvePaths(dir), nil
	})
	p, _ := v.(RuntimePaths)
	return p
}

func resolvePaths(dir string) RuntimePaths {
	if p, err := cli.RunBareJSON[RuntimePaths](dir, "config", "path"); err == nil && p.Config != "" {
		return p
	}
	// 安装布局下 config/ 与 exe 同级；开发时 exe 在 desktop/bin/，
	// 配置在仓库根 config/。主程序不在同目录时回退到仓库布局，
	// 让设置页在开发态也能读到并编辑真实的主配置。
	base := dir
	if _, err := os.Stat(filepath.Join(base, "config", "fast-https.json")); err != nil {
		repo := filepath.Clean(filepath.Join(base, "..", ".."))
		if _, err := os.Stat(filepath.Join(repo, "config", "fast-https.json")); err == nil {
			base = repo
		}
	}
	return RuntimePaths{
		Config:  filepath.Join(base, "config", "fast-https.json"),
		Cert:    filepath.Join(base, "config", "cert"),
		Log:     filepath.Join(base, "logs"),
		Pid:     filepath.Join(base, "fast-https.pid"),
		RootCRT: filepath.Join(base, "httpdoc", "root", "root.crt"),
		State:   filepath.Join(base, "state"),
	}
}

// ------------------------------------------------------------ 服务操作

// RunServerCommand 把纯文本命令（status / version / start / stop / reload）
// 交给主程序，桌面端不自己实现进程管理。
func RunServerCommand(dir string, args ...string) (string, error) {
	return cli.Run(dir, args...)
}

// EnsureServerRunning 复核服务状态，掉了就自动拉起，返回给用户看的补充说明。
//
// 用在配置事务之后：Windows 上事务的 reload 阶段用 GenerateConsoleCtrlEvent
// 给服务进程发信号，而该 API 只能发给整个控制台进程组，服务会被打断
// （用户看到的现象是「点禁用站点，服务停了」）。事务本身成功了（配置已写入、
// 恢复点已建），但服务需要重新拉起才继续可用。
//
// 服务本来就没运行、或拉起失败时都返回空串：不该把「本来就没开」说成故障。
func EnsureServerRunning(dir string) string {
	if running, _ := serverStillRunning(dir); running {
		return ""
	}
	// 给事务的收尾留一点时间，避免误判正在重启中的服务
	time.Sleep(600 * time.Millisecond)
	if running, _ := serverStillRunning(dir); running {
		return ""
	}
	if _, err := StartServer(dir); err != nil {
		return "（服务未能自动恢复，请手动启动：配置已保存，不影响下次启动生效）"
	}
	// 等它真正起来再回报
	for i := 0; i < 10; i++ {
		time.Sleep(300 * time.Millisecond)
		if running, _ := serverStillRunning(dir); running {
			return "（配置已生效，服务已自动恢复）"
		}
	}
	return "（配置已保存，服务正在启动中，稍后刷新状态）"
}

// StartServer 启动服务进程。
//
// Windows 上 `fast-https start` 是前台阻塞的服务进程本身（主程序在
// Windows 不做 daemonize），所以桌面端必须把它彻底分离出去：
//   - 用 CREATE_NEW_PROCESS_GROUP + CREATE_NO_WINDOW 让它独立于桌面端的
//     控制台与作业对象，否则桌面端一退出（或关窗到托盘后被回收）服务会
//     被连带终止，表现为「重启/重载经常失败、状态飘」。
//   - 不继承标准输入输出，避免管道被父进程关掉时子进程收到断管错误。
func StartServer(dir string) (string, error) {
	exe := ServerBinary(dir)
	if _, err := os.Stat(exe); err != nil {
		if _, pathErr := exec.LookPath(exe); pathErr != nil {
			return "", fmt.Errorf("未找到主程序 %s：桌面端需要与主程序放在同一目录", exe)
		}
	}
	cmd := exec.Command(exe, "start")
	cmd.Dir = dir
	setDetached(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return "启动指令已发送，状态会在 2 秒后刷新", nil
}

// StopServer 停止服务，保证「点停止就是真停」：
//
//  1. 转交 stop（pid 文件是相对路径 ./fast-https.pid，按 stop 进程的 CWD
//     解析；桌面端固定 CWD=dir，与 StartServer 一致，正常情况这一步就停掉）
//  2. 用 service status 复核。仍监听说明 stop 没找到 pid（pid 文件缺失或
//     指向别的进程，常见于服务不是桌面端启动、或上次异常退出留下错位 pid），
//     此时按 exe 路径找进程强杀兜底——用户要的是「停」，不是一句「已停止」
//     而站点仍可访问。
func StopServer(dir string) (string, error) {
	// CLI 的 stop 会往 stdout 打 zerolog 的 JSON 行（以及「not running」之类的
	// 提示），不能直接当文案给用户看。这里只把它当成功/失败信号，结果文案
	// 由下面按实际复核结果生成。
	if _, err := cli.Run(dir, "stop"); err != nil {
		return "", fmt.Errorf("停止失败：%v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	if running, ports := serverStillRunning(dir); running {
		killed, killErr := killServerProcesses(dir)
		if killed > 0 && killErr == nil {
			time.Sleep(800 * time.Millisecond)
			if still, _ := serverStillRunning(dir); !still {
				return "服务已停止（stop 未找到 pid，已按进程强停）", nil
			}
		}
		return "", fmt.Errorf("stop 未能停止服务（pid 文件错位），强杀也失败：%s；请手动结束 fast-https.exe 进程",
			strings.Join(ports, " "))
	}
	return "服务已停止", nil
}

// ServerRunning 报告服务当前是否在运行（供 Service 层判断是否需要重载/重启）。
func ServerRunning(dir string) bool {
	running, _ := serverStillRunning(dir)
	return running
}

// WaitServerRunning 轮询等服务起来，超时返回 false。
// 重启类操作后用它确认端口真的可连，避免界面刷新时读到「启动中」的中间态。
func WaitServerRunning(dir string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if running, _ := serverStillRunning(dir); running {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// serverStillRunning 用 service status 判断服务是否仍在监听，返回结论与端口串。
func serverStillRunning(dir string) (bool, []string) {
	data, _, err := cli.RunJSON[any](dir, "service", "status")
	if err != nil {
		return false, nil
	}
	m := cli.AsMap(data)
	st := cli.Str(m, "status")
	if st != "running" && st != "degraded" {
		return false, nil
	}
	var ports []string
	for _, p := range cli.AsSlice(m["ports"]) {
		pm := cli.AsMap(p)
		ports = append(ports, ":"+strconv.Itoa(cli.Int(pm, "port")))
	}
	return true, ports
}

// killServerProcesses 按 exe 路径强杀本实例的 fast-https 进程（兜底路径）。
// 只杀 exe 路径与本实例安装目录一致的进程，不碰别的实例。
func killServerProcesses(dir string) (int, error) {
	exe, err := filepath.Abs(ServerBinary(dir))
	if err != nil {
		return 0, err
	}
	killed := 0
	var lastErr error
	for _, p := range GetProcessesByName(filepath.Base(exe)) {
		if strings.EqualFold(p.Path, exe) || p.Path == "" { // Path 空时（权限不足）也按名杀
			if err := KillProcess(int(p.Pid)); err != nil {
				lastErr = err
				continue
			}
			killed++
		}
	}
	return killed, lastErr
}

// ------------------------------------------------------------ 站点（site list）

// ListSites 转交 site list，并把合并展示的 listen 拆成逐端口行。
func ListSites(dir string) ([]Site, error) {
	data, err := runSiteList(dir)
	if err != nil {
		return nil, err
	}
	return siteRows(cli.AsMap(data)), nil
}

func runSiteList(dir string) (any, error) {
	paths := ResolvePaths(dir)
	return cached("site.list:"+paths.Config, 5*time.Second, func() (any, error) {
		data, _, err := cli.RunJSON[any](dir, "site", "list")
		return data, err
	})
}

// splitListen 把 site list 的合并监听串（"80, 443 ssl h2"）拆成单条监听。
func splitListen(merged string) []string {
	var out []string
	for _, part := range strings.Split(merged, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// portOf 取监听项的端口部分（":443"），不是数字端口返回空串。
func portOf(listen string) string {
	f := strings.Fields(listen)
	if len(f) == 0 {
		return ""
	}
	p := strings.TrimPrefix(f[0], ":")
	if _, err := strconv.Atoi(p); err != nil {
		return ""
	}
	return ":" + p
}

// firstToken 取 server_name 的第一个域名（多域名时作展示与访问 URL 用）。
func firstToken(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// siteRows 把 site list 的 data 映射成界面行：**一个站点文件一行**。
//
// 一个站点常同时监听多个端口（如 "80, 443 ssl h2"），它们属于同一个
// 配置文件、同一个开关、同一次启停。之前按监听拆成多行，界面上看起来是
// 「两个站点」，但两行的启停按钮操作的是同一个文件——点其中一个另一个也变，
// 用户会以为禁用逻辑坏了。这里合并成一行，监听列表整体展示。
func siteRows(data map[string]any) []Site {
	var out []Site
	for _, it := range cli.AsSlice(data["sites"]) {
		r := cli.AsMap(it)
		id := cli.Str(r, "name")
		name := firstToken(cli.Str(r, "server_name"))
		if name == "" {
			name = id
		}
		listens := splitListen(cli.Str(r, "listen"))
		// 该站点是否任意一条监听带 ssl：TLS 站点按整体标记
		tls := false
		for _, l := range listens {
			if strings.Contains(l, "ssl") {
				tls = true
				break
			}
		}
		// Port 取第一条监听的端口（portOf 只认单条监听，不能喂合并串）
		port := ""
		if len(listens) > 0 {
			port = portOf(listens[0])
		}
		out = append(out, Site{
			Id:         id,
			Name:       name,
			Port:       port,
			Listen:     joinPorts(listens),
			Listens:    listens,
			TLS:        tls,
			Source:     cli.Str(r, "file"),
			Enabled:    cli.Bool(r, "enabled"),
			CertStatus: cli.Str(r, "cert_status"),
			Error:      cli.Str(r, "error"),
		})
	}
	return out
}

// joinPorts 把多条监听合并成给界面看的端口串："80, 443 ssl h2"。
func joinPorts(listens []string) string {
	if len(listens) == 0 {
		return ""
	}
	return strings.Join(listens, ", ")
}

// ------------------------------------------------------------ 证书（cert list）

// ListCerts 转交 cert list。
func ListCerts(dir string) ([]Cert, error) {
	data, err := runCertList(dir)
	if err != nil {
		return nil, err
	}
	return certRows(cli.AsMap(data)), nil
}

func runCertList(dir string) (any, error) {
	paths := ResolvePaths(dir)
	return cached("cert.list:"+paths.Config, 15*time.Second, func() (any, error) {
		data, _, err := cli.RunJSON[any](dir, "cert", "list")
		return data, err
	})
}

// certRows 把 cert list 的证书条目映射成界面行。
func certRows(data map[string]any) []Cert {
	var out []Cert
	for _, it := range cli.AsSlice(data["certificates"]) {
		e := cli.AsMap(it)
		c := Cert{
			Domain:      strings.Join(cli.Strs(e, "domains"), " "),
			Subject:     cli.Str(e, "subject"),
			Names:       strings.Join(cli.Strs(e, "names"), ", "),
			Issuer:      cli.Str(e, "issuer"),
			NotAfter:    shortDate(cli.Str(e, "not_after")),
			DaysLeft:    cli.Int(e, "days_left"),
			SelfSigned:  cli.Bool(e, "self_signed"),
			Local:       cli.Bool(e, "local_ca") || cli.Bool(e, "self_signed"),
			Status:      cli.Str(e, "status"),
			AcmeManaged: cli.Bool(e, "acme_managed"),
			File:        cli.Str(e, "file"),
		}
		if c.Domain == "" {
			c.Domain = c.Subject
		}
		c.Note = c.File
		if msg := cli.Str(e, "error"); msg != "" {
			c.Note = msg
		}
		out = append(out, c)
	}
	return out
}

// shortDate 把 RFC3339 收成日期（YYYY-MM-DD）；解析不了原样返回。
func shortDate(s string) string {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

// ------------------------------------------------------------ 概览

// CollectOverview 汇总概览页数据：service status（运行态）+ site list（站点）
// + cert list（最近到期）+ config show（改过未重载提示）。
func CollectOverview(dir string) Overview {
	ov := Overview{
		Version:   desktopVersion,
		CheckedAt: time.Now().Format("15:04:05"),
	}
	paths := ResolvePaths(dir)
	ov.Paths = paths

	// service status：只读查询，服务没在跑也成功（status=stopped）。
	// 拿不到时只把状态标成 unknown，不当成「读取失败」——站点、证书、
	// 路径都是纯磁盘数据，服务没启动完全不影响它们刷出来。
	if data, _, err := cli.RunJSON[any](dir, "service", "status"); err == nil {
		m := cli.AsMap(data)
		ov.Status = cli.Str(m, "status")
		ov.Running = ov.Status == "running" || ov.Status == "degraded"
		ov.PID = cli.Int(m, "pid")
		ov.UptimeSeconds = cli.Int64(m, "uptime_seconds")
		ov.ConfigVersion = cli.Str(m, "config_version")
		if v := cli.Str(m, "version"); v != "" {
			ov.Version = v
		}
		for _, p := range cli.AsSlice(m["ports"]) {
			pm := cli.AsMap(p)
			ov.Ports = append(ov.Ports, ":"+strconv.Itoa(cli.Int(pm, "port")))
		}
		sort.Strings(ov.Ports)
	} else {
		ov.Status = "unknown"
	}

	// site list：站点数与 TLS 站点数（按唯一站点计，不按拆行后的行数）。
	// 站点数据是磁盘上的站点文件，服务没启动也照常列出。
	if data, err := runSiteList(dir); err == nil {
		seen := map[string]bool{}
		for _, s := range siteRows(cli.AsMap(data)) {
			if seen[s.Id] {
				continue
			}
			seen[s.Id] = true
			ov.SiteCount++
			if s.TLS {
				ov.TLSCount++
			}
		}
	} else if ov.Error == "" {
		ov.Error = err.Error()
	}

	// cert list：最近一张到期天数。
	if data, err := runCertList(dir); err == nil {
		for _, c := range certRows(cli.AsMap(data)) {
			if c.DaysLeft > 0 && (ov.NearestCertDays == 0 || c.DaysLeft < ov.NearestCertDays) {
				ov.NearestCertDays = c.DaysLeft
			}
		}
	}

	// config show：config_dirty_hint（仅服务在跑时由主程序给出）。
	if data, err := runConfigShow(dir); err == nil {
		ov.ConfigDirty = cli.Bool(cli.AsMap(data), "config_dirty_hint")
	}
	return ov
}

func runConfigShow(dir string) (any, error) {
	paths := ResolvePaths(dir)
	return cached("config.show:"+paths.Config, 10*time.Second, func() (any, error) {
		data, _, err := cli.RunJSON[any](dir, "config", "show")
		return data, err
	})
}

// ------------------------------------------------------------ 日志（logs query / logs access）

var dateOnlyRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// normalizeBound 把纯日期补成 RFC3339（CLI 只认时长或 RFC3339）。
// 起始日取当天 00:00，截止日取 23:59:59，时区用本机当前偏移。
func normalizeBound(s string, until bool) string {
	s = strings.TrimSpace(s)
	if !dateOnlyRe.MatchString(s) {
		return s
	}
	clock := "T00:00:00"
	if until {
		clock = "T23:59:59"
	}
	return s + clock + time.Now().Format("-07:00")
}

// logsQueryArgs 把查询条件翻译成 logs query 的参数。游标来自已展示的
// 记录，天然比筛选的 until 更早，有游标时直接以其收窄。
func logsQueryArgs(q LogQuery) []string {
	kind := q.Kind
	if kind == "" {
		kind = "access"
	}
	limit := q.Limit
	if limit <= 0 || limit > 1000 {
		limit = 30
	}
	args := []string{"logs", "query", "--kind", kind, "--limit", strconv.Itoa(limit)}
	addFlag := func(name, v string) {
		if strings.TrimSpace(v) != "" {
			args = append(args, name, v)
		}
	}
	addFlag("--level", q.Level)
	addFlag("--status", q.Status)
	addFlag("--domain", q.Host)
	addFlag("--method", q.Method)
	addFlag("--path-prefix", q.PathPrefix)
	addFlag("--reason", q.Reason)
	addFlag("--since", normalizeBound(q.Since, false))
	until := normalizeBound(q.Until, true)
	if q.Cursor != "" {
		until = q.Cursor
	}
	addFlag("--until", until)
	return args
}

// QueryLogs 转交 logs query。logs query 没有 offset 分页：「加载更早」把
// 上一页最早一条的时间当下一页的游标（LogPage.NextUntil）。
func QueryLogs(dir string, q LogQuery) (LogPage, error) {
	data, _, err := cli.RunJSON[any](dir, logsQueryArgs(q)...)
	if err != nil {
		return LogPage{}, err
	}
	m := cli.AsMap(data)
	raw := cli.AsSlice(m["records"])
	var page LogPage
	for _, it := range raw {
		rec := cli.AsMap(it)
		if q.Cursor != "" && cli.Str(rec, "time") == q.Cursor {
			continue // 游标同刻的记录上一页已展示
		}
		page.Lines = append(page.Lines, logEntry(rec))
	}
	page.NextUntil = lastTime(raw)
	page.HasMore = len(raw) > 0 && !cli.Bool(m, "range_complete") &&
		page.NextUntil != "" && page.NextUntil != q.Cursor
	return page, nil
}

// logEntry 把 logs query 的一条记录映射成界面行；Raw 放记录的紧凑 JSON，
// 仅作悬浮提示（CLI 不返回日志原始行）。
func logEntry(rec map[string]any) LogEntry {
	e := LogEntry{
		Time:       cli.Str(rec, "time"),
		Level:      cli.Str(rec, "level"),
		Kind:       cli.Str(rec, "kind"),
		Message:    cli.Str(rec, "message"),
		Method:     cli.Str(rec, "method"),
		Path:       cli.Str(rec, "path"),
		Host:       cli.Str(rec, "host"),
		RemoteAddr: cli.Str(rec, "remote_addr"),
	}
	if v, ok := rec["status"]; ok && v != nil {
		e.Status = fmt.Sprint(v)
	}
	if v, ok := rec["size"]; ok && v != nil {
		e.Size = fmt.Sprint(v)
	}
	if b, err := json.Marshal(rec); err == nil {
		e.Raw = string(b)
	}
	return e
}

// lastTime 取这批记录（由新到旧）里最早一条的时间，作下一页的游标。
func lastTime(records []any) string {
	for i := len(records) - 1; i >= 0; i-- {
		if t := cli.Str(cli.AsMap(records[i]), "time"); t != "" {
			return t
		}
	}
	return ""
}

// TailLog 取日志尾部若干条（状态矩阵的访问日志栏）。
// access / error 走 logs access / logs error，其余走 logs query。
func TailLog(dir, name string, n int) ([]LogEntry, error) {
	if name == "" {
		name = "access"
	}
	switch name {
	case "access", "error", "system", "safe":
	default:
		return nil, errors.New("不支持的日志类型")
	}
	if n <= 0 || n > 200 {
		n = 20
	}
	args := []string{"logs", name, "--limit", strconv.Itoa(n)}
	if name == "system" || name == "safe" {
		args = []string{"logs", "query", "--kind", name, "--limit", strconv.Itoa(n)}
	}
	data, _, err := cli.RunJSON[any](dir, args...)
	if err != nil {
		return nil, err
	}
	var out []LogEntry
	for _, it := range cli.AsSlice(cli.AsMap(data)["records"]) {
		out = append(out, logEntry(cli.AsMap(it)))
	}
	return out, nil
}

// ------------------------------------------------------------ 杂项

// StripJSONC 去掉 // 与 /* */ 注释以及尾逗号。
// 主配置与站点文件都允许 JSONC，标准库的 json 不接受这些写法。
// 搭建候选配置树时要用它解析主配置。
func StripJSONC(b []byte) []byte {
	var out []byte
	inStr, esc, line, block := false, false, false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if line {
			if c == '\n' {
				line = false
				out = append(out, c)
			}
			continue
		}
		if block {
			if c == '*' && i+1 < len(b) && b[i+1] == '/' {
				block = false
				i++
			}
			continue
		}
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(b) {
			if b[i+1] == '/' {
				line = true
				i++
				continue
			}
			if b[i+1] == '*' {
				block = true
				i++
				continue
			}
		}
		out = append(out, c)
	}
	return removeTrailingCommas(out)
}

func removeTrailingCommas(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr, esc := false, false
	for i, c := range b {
		if inStr {
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// EtcPath 复刻主程序 absETCPath 的常见情形：相对路径以主配置所在目录为基址。
// 旧配置里还会出现 "./config/xxx" 这种带前缀的写法，一并兼容。
// 搭建候选配置树解析 http.include 时使用。
func EtcPath(etcDir, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	clean := filepath.Clean(p)
	clean = strings.TrimPrefix(clean, "config"+string(filepath.Separator))
	clean = strings.TrimPrefix(clean, "."+string(filepath.Separator))
	return filepath.Join(etcDir, clean)
}

// WriteNote 是写操作结果文案里关于「生效」的平台提示。
// Windows 上热重载信号（CTRL_BREAK）到不了别的控制台，配置事务里的
// reload 可能没打进去；如实提示，别把「已写入」说成「已生效」。
func WriteNote() string {
	if runtime.GOOS == "windows" {
		return "。配置已通过事务提交；Windows 上热重载有限制，若改动未生效，点「重载配置」会自动重启服务"
	}
	return "。配置事务已校验并自动重载，无需手动刷新。"
}

// OpenPath 用系统默认方式打开文件或目录。
func OpenPath(path string) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("打开 %s 失败：%w", path, err)
	}
	go func() { _ = cmd.Wait() }()
	return "已打开 " + path, nil
}
