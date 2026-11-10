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
	if len(p.names) > 1 {
		plan = append(plan, []string{
			"site", "update", p.names[0],
			"--server-name", strings.Join(p.names, " "),
			"--yes",
		})
	}
	return plan, nil
}

// runSite 统一执行站点 CLI：补 --config（配置树路径由主程序给出，不依赖
// CWD），失败时把封套错误翻译成人话（cli.FriendlyError）。
func runSite(dir string, args ...string) (any, error) {
	paths := view.ResolvePaths(dir)
	full := append([]string{}, args...)
	if paths.Config != "" {
		full = append(full, "--config", paths.Config)
	}
	data, _, err := cli.RunJSON[any](dir, full...)
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
	return txnMessage(data, word, name), nil
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
	if !changed {
		return nil, fmt.Errorf("没有需要保存的改动")
	}
	args = append(args, "--yes")
	return args, nil
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
