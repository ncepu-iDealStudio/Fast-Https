package configops

// 设置页是「按字段填表」，不是编辑整份 JSON：
// Go 侧维护字段清单（纯 UI 元数据），前端按分组通用渲染。
//
// 读取转交 config show（敏感值由主程序脱敏，保存只回写被改动的字段，
// 脱敏值不触碰就不会被写回）；保存搭建候选配置树后转交 config apply 的
// 配置事务——校验、差异、恢复点、切换、reload、失败自动回滚，与命令行
// 语义一致。桌面端只准备候选，不直接改线上配置文件。

import (
	"bytes"
	"encoding/json"
	"fast-https-desktop/internal/cli"
	"fast-https-desktop/internal/model"
	"fast-https-desktop/internal/view"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ConfigField = model.ConfigField
type ConfigForm = model.ConfigForm

type fieldSpec struct {
	path, label, group, typ, help string
	options                       []string
}

// 字段名与 config/schema.go、config/config.go、config/acme.go 保持一致，
// 包括历史拼写 http.blaklist。http.server[] 与 acme.cas 结构复杂，不进表单。
var configSpecs = []fieldSpec{
	{"pid", "pid 文件路径", "基础", "text", "预留字段，通常留空", nil},
	{"log_root", "日志根目录", "基础", "text", "预留字段，通常留空", nil},

	{"security.host_check", "Host 校验", "安全", "select", "off 回落首个 server；warn 只记录；enforce 返回 421", []string{"off", "warn", "enforce"}},
	{"security.headers.nosniff", "X-Content-Type-Options", "安全", "bool", "写入 nosniff 响应头", nil},
	{"security.headers.hsts", "HSTS", "安全", "text", "留空关闭；例 max-age=31536000; includeSubDomains。误开会让 HTTP 站点被浏览器锁死", nil},
	{"security.headers.x_frame_options", "X-Frame-Options", "安全", "select", "留空关闭", []string{"", "DENY", "SAMEORIGIN"}},
	{"security.headers.referrer_policy", "Referrer-Policy", "安全", "select", "留空关闭", []string{"", "no-referrer", "no-referrer-when-downgrade", "origin", "origin-when-cross-origin", "same-origin", "strict-origin", "strict-origin-when-cross-origin", "unsafe-url"}},
	{"security.headers.content_security_policy", "Content-Security-Policy", "安全", "textarea", "留空关闭；策略较长时可分行写", nil},

	{"waf.mode", "WAF 模式", "WAF", "select", "enforce 阻断 / monitor 只记录 / disabled 不启用", []string{"", "enforce", "monitor", "disabled"}},

	{"acme.enabled", "启用自动签发", "ACME", "bool", "关闭后走静态证书", nil},
	{"acme.email", "ACME 邮箱", "ACME", "email", "用于注册 ACME 账户", nil},
	{"acme.staging", "使用 staging 环境", "ACME", "bool", "测试时避免触发正式 CA 速率限制", nil},
	{"acme.challenge", "挑战方式", "ACME", "multiselect", "至少选一个；不在列表里的自定义值会保留", []string{"http-01", "tls-alpn-01", "dns-01"}},
	{"acme.renew_before_days", "提前续期天数", "ACME", "number", "0 表示用默认 30 天", nil},
	{"acme.fallback_self_signed", "证书失败回落自签", "ACME", "bool", "默认回落；显式 false 则不回落", nil},
	{"acme.storage", "ACME 缓存目录", "ACME", "text", "留空用默认路径", nil},

	{"http.include", "include 目录 / 文件", "HTTP", "list", "每行一个，站点文件通常放在 conf.d", nil},
	{"http.default_type", "默认 MIME 类型", "HTTP", "text", "", nil},
	{"http.servers_limit.max_header_size", "请求头上限（字节）", "HTTP", "number", "", nil},
	{"http.servers_limit.max_body_size", "请求体上限（字节）", "HTTP", "number", "", nil},
	{"http.servers_limit.limit", "限流 rate", "HTTP", "number", "0 表示不限", nil},
	{"http.servers_limit.burst", "限流 burst", "HTTP", "number", "0 表示不限", nil},
	{"http.blaklist", "黑名单", "HTTP", "list", "历史键名 blaklist，每行一个", nil},
	{"http.log_format", "访问日志字段", "HTTP", "multiselect", "按需勾选，顺序即日志字段顺序", []string{"ip_port", "time", "type", "method", "path", "host", "status", "size", "user_agent"}},
}

// engine（is_master / id / register_port / slave_*）是 schema 里为兼容存量配置
// 保留的集群占位，当前没有任何模块读取，项目也未启用集群，因此不进表单。
// 若配置里仍带 engine 段，会列入 ConfigForm.Preserved 并在保存时原样保留。
var configTopLevel = map[string]bool{
	"pid": true, "log_root": true,
	"waf": true, "acme": true, "security": true, "http": true,
}

// ReadConfigForm 转交 config show 取表单字段值（敏感值已脱敏）；
// 文件大小、修改时间与可写标记是纯 UI 元数据，本地 stat。
func ReadConfigForm(dir string) (ConfigForm, error) {
	paths := view.ResolvePaths(dir)
	form := ConfigForm{Path: paths.Config}
	info, err := os.Stat(paths.Config)
	if err != nil {
		return form, fmt.Errorf("读不到主配置 %s：%w", paths.Config, err)
	}
	form.Size = info.Size()
	form.Modified = info.ModTime().Format("2006-01-02 15:04:05")
	if f, err := os.OpenFile(paths.Config, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		form.Writable = true
	}

	data, _, err := cli.RunJSON[any](dir, append([]string{"config", "show"}, view.ConfigArg(paths)...)...)
	if err != nil {
		return form, err
	}
	root := cli.AsMap(cli.AsMap(data)["config"])
	for k := range root {
		if !configTopLevel[k] {
			form.Preserved = append(form.Preserved, k)
		}
	}
	sort.Strings(form.Preserved)
	for _, s := range configSpecs {
		f := ConfigField{Path: s.path, Label: s.label, Group: s.group, Type: s.typ, Help: s.help, Options: s.options}
		if v, ok := lookupPath(root, s.path); ok {
			f.Value = formatFieldValue(v, s.typ)
		} else {
			f.Missing = true
			f.Value = defaultFieldValue(s)
		}
		form.Fields = append(form.Fields, f)
	}
	return form, nil
}

// SaveConfigForm 只把「被改动的字段」写进候选配置树，再转交 config apply
// 的配置事务。事务负责校验、恢复点、切换与 reload，失败自动回滚。
func SaveConfigForm(dir string, values map[string]string) (string, error) {
	paths := view.ResolvePaths(dir)
	raw, err := os.ReadFile(paths.Config)
	if err != nil {
		return "", fmt.Errorf("读不到主配置 %s：%w", paths.Config, err)
	}
	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(view.StripJSONC(raw)))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return "", fmt.Errorf("主配置不是合法的 JSON 对象：%w", err)
	}
	changed := 0
	for _, s := range configSpecs {
		text, ok := values[s.path]
		if !ok {
			continue
		}
		v, err := parseFieldValue(s, text)
		if err != nil {
			return "", fmt.Errorf("%s：%w", s.label, err)
		}
		setPath(root, s.path, v)
		changed++
	}
	if changed == 0 {
		return "", fmt.Errorf("没有需要保存的改动")
	}

	cand, err := buildCandidate(paths, root)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(cand) }()

	data, _, err := cli.RunJSON[any](dir, append([]string{"config", "apply", "--candidate", cand, "--yes"}, view.ConfigArg(paths)...)...)
	if err != nil {
		return "", err
	}
	m := cli.AsMap(data)
	msg := fmt.Sprintf("已保存 %d 个字段并应用（配置事务完成）", changed)
	var extra []string
	if id := cli.Str(m, "operation_id"); id != "" {
		extra = append(extra, "操作 "+id)
	}
	if v := cli.Str(m, "version"); v != "" {
		extra = append(extra, "配置版本 "+v)
	}
	if r := cli.Str(m, "restore_point"); r != "" {
		extra = append(extra, "恢复点 "+r)
	}
	if len(extra) > 0 {
		msg += "（" + strings.Join(extra, "，") + "）"
	}
	return msg + view.WriteNote(), nil
}

// buildCandidate 复刻主程序 siteops.CloneLiveTree 搭一棵候选配置树：
// 改过字段的 fast-https.json + conf.d 下的站点文件副本（含停用文件）。
// 事务引擎按相对路径把候选树整体切换到线上，因此站点文件必须放在
// conf.d 下；http.include 带绝对路径时改写成 ["conf.d"]，与 site add
// 的事务行为一致。
func buildCandidate(paths model.RuntimePaths, root map[string]any) (string, error) {
	cand, err := os.MkdirTemp("", "fast-https-cand-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(cand)
		}
	}()
	if err := os.MkdirAll(filepath.Join(cand, "conf.d"), 0o755); err != nil {
		return "", err
	}
	etcDir := filepath.Dir(paths.Config)

	var includes []string
	needRewrite := false
	if http, ok := root["http"].(map[string]any); ok {
		for _, x := range cli.AsSlice(http["include"]) {
			s, _ := x.(string)
			if strings.TrimSpace(s) == "" {
				continue
			}
			if filepath.IsAbs(strings.TrimSpace(s)) {
				needRewrite = true
			}
			includes = append(includes, s)
		}
	}
	for _, inc := range includes {
		target := view.EtcPath(etcDir, inc)
		for _, f := range collectTreeFiles(target) {
			data, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			dst := filepath.Join(cand, "conf.d", filepath.Base(f))
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return "", err
			}
		}
	}
	if needRewrite {
		setPath(root, "http.include", []string{"conf.d"})
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(cand, "fast-https.json"), append(out, '\n'), 0o644); err != nil {
		return "", err
	}
	ok = true
	return cand, nil
}

// collectTreeFiles 收集 include 目标下的站点文件（*.json 与停用的
// *.json.disabled），目标是单个文件时直接返回它。
func collectTreeFiles(target string) []string {
	info, err := os.Stat(target)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		name := strings.ToLower(filepath.Base(target))
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.disabled") {
			return []string{target}
		}
		return nil
	}
	var files []string
	_ = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.disabled") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func lookupPath(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, k := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func setPath(root map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	cur := root
	for i, k := range parts {
		if i == len(parts)-1 {
			cur[k] = v
			return
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
}

func formatFieldValue(v any, typ string) string {
	if typ == "list" || typ == "multiselect" {
		if arr, ok := v.([]any); ok {
			items := make([]string, 0, len(arr))
			for _, x := range arr {
				items = append(items, fmt.Sprint(x))
			}
			return strings.Join(items, "\n")
		}
	}
	if typ == "bool" {
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b)
		}
	}
	switch n := v.(type) {
	case string:
		return n
	case float64:
		// JSON 数字默认解码成 float64，大数会被 %v 打成科学计数法。
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case json.Number:
		return n.String()
	}
	return fmt.Sprint(v)
}

func defaultFieldValue(s fieldSpec) string {
	switch s.typ {
	case "number":
		return "0"
	case "bool":
		return "false"
	case "select":
		if len(s.options) > 0 {
			return s.options[0]
		}
	}
	return ""
}

func parseFieldValue(s fieldSpec, raw string) (any, error) {
	switch s.typ {
	case "bool":
		if raw != "true" && raw != "false" {
			return nil, fmt.Errorf("布尔值只能是 true/false")
		}
		return raw == "true", nil
	case "number":
		t := strings.TrimSpace(raw)
		if t == "" {
			t = "0"
		}
		n, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("不是合法数字：%q", raw)
		}
		return json.Number(strconv.FormatFloat(n, 'f', -1, 64)), nil
	case "list", "multiselect":
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
		items := make([]string, 0, len(parts))
		for _, x := range parts {
			if x = strings.TrimSpace(x); x != "" {
				items = append(items, x)
			}
		}
		return items, nil
	case "select":
		t := strings.TrimSpace(raw)
		for _, o := range s.options {
			if o == t {
				return t, nil
			}
		}
		return nil, fmt.Errorf("取值必须是 %s", strings.Join(s.options, " / "))
	default:
		return raw, nil
	}
}
