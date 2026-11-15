package siteops

// 站点写操作全部转交主程序 site 命令的配置事务（校验、恢复点、reload、
// 可回滚），桌面端不自己改站点文件；读取走 site show。这里只做三件纯 UI
// 的事：表单校验（快速反馈，权威校验仍在 CLI）、弹窗字段与命令 flag 的
// 翻译、结果文案。

import (
	"encoding/json"
	"fast-https-desktop/internal/cli"
	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/view"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SiteInput 是「添加 / 编辑站点」弹窗提交的字段。
type SiteInput = model.SiteInput

// SiteDetail 是详情弹窗一次拉取的内容。
type SiteDetail = model.SiteDetail

// parsedSite 是 SiteInput 校验并归一化后的结果。
type parsedSite struct {
	names   []string
	typ     string
	root    string
	pass    string
	listens []string
	tls     bool
}

// parseSiteInput 做快速校验并归一化。域名即 CLI 的站点名（文件名主干），
// 多域名时空格分隔（nginx 风格），第一个是主域名。
func parseSiteInput(in SiteInput) (parsedSite, error) {
	var p parsedSite
	p.names = strings.Fields(in.Name)
	if len(p.names) == 0 {
		return p, fmt.Errorf("server_name 不能为空")
	}
	for _, name := range p.names {
		if strings.ContainsAny(name, "/\\:@#?&=+$,;!*'\"<>[]{}|") {
			return p, fmt.Errorf("域名 %q 包含非法字符", name)
		}
	}
	p.typ = strings.TrimSpace(in.Type)
	if p.typ != "local" && p.typ != "proxy" {
		return p, fmt.Errorf("站点类型只能是 local 或 proxy")
	}
	p.root = strings.TrimSpace(in.Root)
	p.pass = strings.TrimSpace(in.ProxyPass)
	if p.typ == "local" && p.root == "" {
		return p, fmt.Errorf("静态站点需要填写 root（站点根目录）")
	}
	if p.typ == "proxy" {
		if p.pass == "" {
			return p, fmt.Errorf("反向代理需要填写 proxy_pass")
		}
		u, err := url.Parse(p.pass)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return p, fmt.Errorf("proxy_pass 必须是 http:// 或 https:// 开头的完整地址")
		}
	}
	for _, raw := range strings.FieldsFunc(in.Listen, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		fields := strings.Fields(raw)
		port, err := strconv.Atoi(strings.TrimPrefix(fields[0], ":"))
		if err != nil || port < 1 || port > 65535 {
			return p, fmt.Errorf("监听项 %q 不是合法端口（1-65535）", raw)
		}
		p.listens = append(p.listens, raw)
		if strings.Contains(raw, "ssl") {
			p.tls = true
		}
	}
	if len(p.listens) == 0 {
		return p, fmt.Errorf("listen 至少需要一个监听端口")
	}
	return p, nil
}

// txnMessage 从事务结果封套里取 operation_id / version，拼结果文案。
func txnMessage(data any, verb, name string) string {
	m := cli.AsMap(data)
	msg := fmt.Sprintf("已%s站点 %s", verb, name)
	var extra []string
	if id := cli.Str(m, "operation_id"); id != "" {
		extra = append(extra, "操作 "+id)
	}
	if v := cli.Str(m, "version"); v != "" {
		extra = append(extra, "配置版本 "+v)
	}
	if len(extra) > 0 {
		msg += "（" + strings.Join(extra, "，") + "）"
	}
	return msg + view.WriteNote()
}

// planAdd 把弹窗输入翻译成 CLI 参数：site add 建站（域名即文件名），
// 多域名时再用 site update --server-name 补全 server_name。
func planAdd(in SiteInput) ([][]string, error) {
	p, err := parseSiteInput(in)
	if err != nil {
		return nil, err
	}
	add := []string{"site", "add", "--domain", p.names[0]}
	for _, l := range p.listens {
		add = append(add, "--listen", l)
	}
	if p.typ == "proxy" {
		add = append(add, "--proxy-pass", p.pass)
	} else {
		add = append(add, "--root", p.root)
	}
	if p.tls {
		add = append(add, "--tls")
	}
	add = append(add, "--yes")
	plan := [][]string{add}

	// site add 不接受 zip / limit / auth / cache，所以建好之后补一条
	// site update 把它们写进去。与上面多域名补全是同一套路：先创建，
	// 再用受控更新补齐；中途失败会明确告诉用户站点已建成。
	var extra []string
	if len(p.names) > 1 {
		extra = append(extra, "--server-name", strings.Join(p.names, " "))
	}
	if hasAdvanced(in) {
		extra = append(extra, advancedArgs(in)...)
	}
	if len(extra) > 0 {
		plan = append(plan, append([]string{"site", "update", p.names[0]}, append(extra, "--yes")...))
	}
	return plan, nil
}

// runSite 统一执行站点 CLI：原样转交 args（不注入任何额外参数——不同命令
// 支持的 flag 各不相同，统一加参数会让命令报 unknown flag），失败时把
// 封套错误翻译成人话（cli.FriendlyError）。
func runSite(dir string, args ...string) (any, error) {
	data, _, err := cli.RunJSON[any](dir, args...)
	if err != nil {
		return nil, fmt.Errorf("%s", cli.FriendlyError(err, strings.Join(args, " ")))
	}
	return data, nil
}

// AddSite 受控新增站点：转交 site add（多域名再补一条 site update）。
func AddSite(dir string, in SiteInput) (string, error) {
	plan, err := planAdd(in)
	if err != nil {
		return "", err
	}
	primary := strings.Fields(in.Name)[0]
	var msgs []string
	for _, args := range plan {
		data, err := runSite(dir, args...)
		if err != nil {
			if len(msgs) > 0 {
				return "", fmt.Errorf("站点已创建但补充多域名失败：%w", err)
			}
			return "", err
		}
		msgs = append(msgs, txnMessage(data, "创建", primary))
	}
	if len(msgs) > 1 {
		return msgs[0] + "多域名已通过 site update 补全。", nil
	}
	return msgs[0], nil
}

// DeleteSite 受控删除站点配置（不删内容目录与证书），转交 site delete。
func DeleteSite(dir, id string) (string, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if id == "" {
		return "", fmt.Errorf("缺少站点名")
	}
	data, err := runSite(dir, "site", "delete", id, "--yes")
	if err != nil {
		return "", err
	}
	return txnMessage(data, "删除", id), nil
}

// SetSiteEnabled 转交 site enable/disable 的配置事务。
// 桌面端不改文件名：事务会处理校验、恢复点与 reload，和命令行语义一致。
//
// Windows 特殊处理：事务的 reload 阶段用 GenerateConsoleCtrlEvent 给服务
// 进程发信号，而该 API 只能发给整个控制台进程组——服务会被打断（表现为
// 点「禁用」后服务停了，操作记录停在 recovery_required / reloaded）。
// 所以事务结束后复核服务状态，掉了就自动拉起来，让「禁用站点」不影响
// 服务可用性。
func SetSiteEnabled(dir, id string, enabled bool) (string, error) {
	name := strings.TrimSpace(id)
	name = strings.TrimSuffix(name, ".disabled")
	name = strings.TrimSuffix(name, ".json")
	if name == "" {
		return "", fmt.Errorf("缺少站点名")
	}
	action, word := "disable", "停用"
	if enabled {
		action, word = "enable", "启用"
	}
	data, err := runSite(dir, "site", action, name)
	if err != nil {
		return "", err
	}
	msg := txnMessage(data, word, name)
	if note := view.EnsureServerRunning(dir); note != "" {
		msg += note
	}
	return msg, nil
}

// ReadSiteDetail 转交 site show：表单字段 + 只读信息（证书、首页、原文）。
// 内联在主配置 http.server[] 里的站点返回 Editable=false 加提示。
func ReadSiteDetail(dir, id string) (SiteDetail, error) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if id == "" {
		return SiteDetail{}, fmt.Errorf("缺少站点名")
	}
	paths := view.ResolvePaths(dir)
	data, err := runSite(dir, "site", "show", id)
	if err != nil {
		return SiteDetail{}, err
	}
	m := cli.AsMap(data)
	srv := cli.AsMap(m["server"])
	d := SiteDetail{
		Id:      id,
		Raw:     prettyJSON(srv),
		Name:    cli.Str(srv, "server_name"),
		Cert:    cli.Str(srv, "ssl_certificate"),
		CertKey: cli.Str(srv, "ssl_certificate_key"),
	}
	d.Source = cli.Str(m, "source")
	if rel, err := filepath.Rel(filepath.Dir(paths.Config), d.Source); err == nil && !strings.HasPrefix(rel, "..") {
		d.Path = filepath.ToSlash(rel)
	} else {
		d.Path = d.Source
	}
	d.Listen = formatListenValue(srv["listen"])
	if locs := cli.AsSlice(srv["location"]); len(locs) > 0 {
		if loc, ok := locs[0].(map[string]any); ok {
			d.Type = cli.Str(loc, "type")
			d.Root = cli.Str(loc, "root")
			d.ProxyPass = cli.Str(loc, "proxy_pass")
			d.Index = strings.Join(cli.Strs(loc, "index"), " ")
			fillAdvanced(&d, loc)
		}
	}
	if d.Type == "" {
		d.Type = "local"
	}
	d.Editable = !sameFile(d.Source, paths.Config)
	if !d.Editable {
		d.Note = "该站点内联在主配置 http.server[] 里，请到设置页的主配置表单里修改"
	}
	return d, nil
}

// fillAdvanced 把 site show 的 location 高级配置填进详情。
// auth.pswd 已被 site show 脱敏成 ***，原样保留：前端留空即不改密码。
func fillAdvanced(d *SiteDetail, loc map[string]any) {
	d.Zip = cli.Strs(loc, "zip")

	if limit := cli.AsMap(loc["limit"]); limit != nil {
		d.LimitRate = cli.Int(limit, "rate")
		d.LimitBurst = cli.Int(limit, "burst")
		d.LimitMem = cli.Int(limit, "mem")
		d.LimitNodelay = cli.Bool(limit, "nodelay")
	}

	if auth := cli.AsMap(loc["auth"]); auth != nil {
		d.AuthOn = cli.Str(auth, "type") != "" || cli.Str(auth, "user") != ""
		d.AuthUser = cli.Str(auth, "user")
		d.AuthPass = cli.Str(auth, "pswd")
		d.AuthRealm = cli.Str(auth, "realm")
	}

	if cache := cli.AsMap(loc["proxy_cache"]); cache != nil {
		d.CachePath = cli.Str(cache, "path")
		d.CacheValid = cli.Strs(cache, "valid")
		d.CacheMaxSize = cli.Int(cache, "max_size")
	}
}

// planUpdate 比较弹窗当前值与 site show 的原值，翻译成 site update 的参数。
// 至少要有一个实际变更；listen 文本是 TLS 的唯一事实来源（含 ssl 即启用）。
func planUpdate(id string, cur SiteDetail, in SiteInput) ([]string, error) {
	p, err := parseSiteInput(in)
	if err != nil {
		return nil, err
	}
	args := []string{"site", "update", id}
	changed := false

	if strings.Join(p.names, " ") != strings.Join(strings.Fields(cur.Name), " ") {
		args = append(args, "--server-name", strings.Join(p.names, " "))
		changed = true
	}
	newListen := strings.Join(p.listens, ",")
	oldListen := strings.Join(splitListenText(cur.Listen), ",")
	if newListen != oldListen {
		for _, l := range p.listens {
			args = append(args, "--listen", l)
		}
		changed = true
	}
	if p.typ == "proxy" {
		if p.typ != cur.Type || p.pass != cur.ProxyPass {
			args = append(args, "--proxy-pass", p.pass)
			changed = true
		}
	} else if p.typ != cur.Type || p.root != cur.Root {
		args = append(args, "--root", p.root)
		changed = true
	}
	if advArgs, advChanged := planAdvanced(cur, in); advChanged {
		args = append(args, advArgs...)
		changed = true
	}
	if !changed {
		return nil, fmt.Errorf("没有需要保存的改动")
	}
	args = append(args, "--yes")
	return args, nil
}

// planAdvanced 把弹窗的高级配置翻译成 --zip / --limit-* / --auth-* / --cache-*。
//
// 只在真的有变化时才产出参数：没动过的块完全不出现在命令行里，CLI 那边的
// 指针就是 nil，配置文件里对应键原样保留。密码按「留空或 *** 视为不改」
// 处理，避免把脱敏值当成新口令写回去。
func planAdvanced(cur SiteDetail, in SiteInput) ([]string, bool) {
	var args []string
	changed := false

	if !sameStringSet(cur.Zip, in.Zip) {
		args = append(args, zipArgs(in.Zip)...)
		changed = true
	}

	if cur.LimitRate != in.LimitRate || cur.LimitBurst != in.LimitBurst ||
		cur.LimitMem != in.LimitMem || cur.LimitNodelay != in.LimitNodelay {
		args = append(args, limitArgs(in)...)
		changed = true
	}

	if authArgs, authChanged := planAuth(cur, in); authChanged {
		args = append(args, authArgs...)
		changed = true
	}

	if cur.CachePath != in.CachePath || !sameStringSet(cur.CacheValid, in.CacheValid) ||
		cur.CacheMaxSize != in.CacheMaxSize {
		args = append(args, cacheArgs(in)...)
		changed = true
	}

	return args, changed
}

// advancedArgs 把高级配置**全部**翻成参数，不做「和当前值比」的过滤。
//
// 新建站点要用它：site add 不接受这四个字段，只能在建好之后补一条
// site update；那时没有「当前详情」可比，传什么就该写什么。
func advancedArgs(in SiteInput) []string {
	var args []string
	args = append(args, zipArgs(in.Zip)...)
	args = append(args, limitArgs(in)...)
	args = append(args, authArgs(in)...)
	args = append(args, cacheArgs(in)...)
	return args
}

// hasAdvanced 报告用户是否在弹窗里填过任何高级配置。
// 新建站点据此决定要不要追加那条 site update。
func hasAdvanced(in SiteInput) bool {
	return len(in.Zip) > 0 || in.LimitRate > 0 || in.LimitBurst > 0 || in.LimitMem > 0 ||
		in.LimitNodelay || in.AuthOn || strings.TrimSpace(in.CachePath) != ""
}

// zipArgs 生成响应压缩参数。空列表用 --zip= 表示清除，
// 因为 CLI 只认「显式给了空值」为删除、没给为不改。
func zipArgs(zip []string) []string {
	var args []string
	if len(zip) == 0 {
		return []string{"--zip="}
	}
	for _, z := range zip {
		if s := strings.TrimSpace(z); s != "" {
			args = append(args, "--zip", s)
		}
	}
	if len(args) == 0 {
		return []string{"--zip="}
	}
	return args
}

// limitArgs 生成限流参数。三个数字全为 0 且没开 nodelay 时，
// 仍显式写 0，让 CLI 删掉整个 limit 键而不是留一个全零对象。
func limitArgs(in SiteInput) []string {
	args := []string{
		"--limit-rate", strconv.Itoa(in.LimitRate),
		"--limit-burst", strconv.Itoa(in.LimitBurst),
		"--limit-mem", strconv.Itoa(in.LimitMem),
	}
	if in.LimitNodelay {
		args = append(args, "--limit-nodelay")
	}
	return args
}

// cacheArgs 生成反向代理缓存参数。目录留空表示清除整块 proxy_cache。
func cacheArgs(in SiteInput) []string {
	path := strings.TrimSpace(in.CachePath)
	if path == "" {
		return []string{"--cache-path="}
	}
	args := []string{"--cache-path", path}
	for _, v := range in.CacheValid {
		if s := strings.TrimSpace(v); s != "" {
			args = append(args, "--cache-valid", s)
		}
	}
	if in.CacheMaxSize > 0 {
		args = append(args, "--cache-max-size", strconv.Itoa(in.CacheMaxSize))
	}
	return args
}

// planAuth 处理 Basic 认证：关掉时清空四个值，开着时只在密码真的变了才传
// --auth-pass——留空或 *** 都不传，CLI 侧沿用文件里的原口令。
func planAuth(cur SiteDetail, in SiteInput) ([]string, bool) {
	if !in.AuthOn {
		if !cur.AuthOn {
			return nil, false
		}
		return []string{"--auth-type="}, true
	}
	pass := strings.TrimSpace(in.AuthPass)
	keepPass := pass == "" || pass == "***"
	same := cur.AuthOn && cur.AuthUser == in.AuthUser &&
		strings.TrimSpace(cur.AuthRealm) == strings.TrimSpace(in.AuthRealm) && keepPass
	if same {
		return nil, false
	}
	return authArgs(in), true
}

// authArgs 生成 Basic 认证参数，不做「和当前值比」的过滤（新建站点用）。
func authArgs(in SiteInput) []string {
	if !in.AuthOn {
		return []string{"--auth-type="}
	}
	args := []string{"--auth-type", "basic", "--auth-user", in.AuthUser}
	pass := strings.TrimSpace(in.AuthPass)
	if pass != "" && pass != "***" {
		args = append(args, "--auth-pass", pass)
	}
	if realm := strings.TrimSpace(in.AuthRealm); realm != "" {
		args = append(args, "--auth-realm", realm)
	}
	return args
}

// sameStringSet 按集合比较两个字符串切片，忽略顺序与首尾空白。
func sameStringSet(a, b []string) bool {
	clean := func(in []string) []string {
		out := make([]string, 0, len(in))
		for _, raw := range in {
			if v := strings.ToLower(strings.TrimSpace(raw)); v != "" {
				out = append(out, v)
			}
		}
		sort.Strings(out)
		return out
	}
	ca, cb := clean(a), clean(b)
	if len(ca) != len(cb) {
		return false
	}
	for i := range ca {
		if ca[i] != cb[i] {
			return false
		}
	}
	return true
}

// SaveSiteDetail 受控更新站点的白名单字段（server_name / listen / root /
// proxy_pass），转交 site update；未涉及的 location 与未知键由 CLI 保留。
func SaveSiteDetail(dir string, in SiteInput) (string, error) {
	id := strings.TrimSuffix(strings.TrimSpace(in.Id), ".json")
	if id == "" {
		return "", fmt.Errorf("缺少站点名")
	}
	cur, err := ReadSiteDetail(dir, id)
	if err != nil {
		return "", err
	}
	if !cur.Editable {
		return "", fmt.Errorf("主配置里的内联站点请在设置页修改")
	}
	args, err := planUpdate(id, cur, in)
	if err != nil {
		return "", err
	}
	data, err := runSite(dir, args...)
	if err != nil {
		return "", err
	}
	return txnMessage(data, "更新", id), nil
}

// splitListenText 把弹窗的监听输入拆成单条监听。
func splitListenText(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// formatListenValue 把 site show 的 listen（数字 / 字符串 / 数组）拼成
// 弹窗输入框的写法："80, 443 ssl h2"。
func formatListenValue(v any) string {
	switch t := v.(type) {
	case []any:
		items := make([]string, 0, len(t))
		for _, x := range t {
			items = append(items, fmt.Sprint(x))
		}
		return strings.Join(items, ", ")
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func sameFile(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func prettyJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
