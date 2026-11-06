package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 设置页是「按字段填表」，不是编辑整份 JSON：
// Go 侧维护字段清单，前端按分组通用渲染，保存时只回写被改动的字段，
// 未纳入表单的字段（http.server[]、acme.cas、未知顶层键）原样保留。

type ConfigField struct {
	Path    string   `json:"path"`
	Label   string   `json:"label"`
	Group   string   `json:"group"`
	Type    string   `json:"type"` // text | number | bool | select | list
	Value   string   `json:"value"`
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
	Missing bool     `json:"missing"`
}

type ConfigForm struct {
	Path      string        `json:"path"`
	Fields    []ConfigField `json:"fields"`
	Size      int64         `json:"size"`
	Modified  string        `json:"modified"`
	Writable  bool          `json:"writable"`
	Preserved []string      `json:"preserved"`
}

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

func readConfigForm(p string) (ConfigForm, error) {
	form := ConfigForm{Path: p}
	info, err := os.Stat(p)
	if err != nil {
		return form, fmt.Errorf("读不到主配置 %s：%w", p, err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return form, fmt.Errorf("读不到主配置 %s：%w", p, err)
	}
	var root map[string]any
	if err := json.Unmarshal(stripJSONC(raw), &root); err != nil {
		return form, fmt.Errorf("主配置不是合法的 JSON 对象：%w", err)
	}
	form.Size = info.Size()
	form.Modified = info.ModTime().Format("2006-01-02 15:04:05")
	if f, err := os.OpenFile(p, os.O_WRONLY, 0); err == nil {
		_ = f.Close()
		form.Writable = true
	}
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

// saveConfigForm 只把「被改动的字段」写回原 JSON，再交给 saveConfigFile
// 做语法校验、备份与原子替换。表单保存会重新格式化文件，JSONC 注释会丢失，
// 原文件完整保留在 .bak 里。
func saveConfigForm(p string, values map[string]string) (string, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", fmt.Errorf("读不到主配置 %s：%w", p, err)
	}
	var root map[string]any
	if err := json.Unmarshal(stripJSONC(raw), &root); err != nil {
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
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	return saveConfigFile(p, string(out)+"\n")
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
	if s, ok := v.(string); ok {
		return s
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
		if _, err := strconv.ParseFloat(t, 64); err != nil {
			return nil, fmt.Errorf("不是合法数字：%q", raw)
		}
		return json.Number(t), nil
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

// saveConfigFile 校验 JSON 对象后写回：先留 .bak，再写临时文件原子替换。
func saveConfigFile(p, content string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC([]byte(content)), &obj); err != nil {
		return "", fmt.Errorf("配置不是合法的 JSON 对象：%w", err)
	}
	if len(obj) == 0 {
		return "", fmt.Errorf("配置是空对象，拒绝写入")
	}
	dir := filepath.Dir(p)
	perm := os.FileMode(0644)
	if info, err := os.Stat(p); err == nil {
		perm = info.Mode().Perm()
		if orig, err := os.ReadFile(p); err == nil {
			_ = os.WriteFile(p+".bak", orig, perm)
		}
	}
	tmp, err := os.CreateTemp(dir, ".fast-https-*.tmp")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败：%w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, p); err != nil {
		return "", fmt.Errorf("替换主配置失败：%w", err)
	}
	return fmt.Sprintf("已保存到 %s（原文件备份为 %s.bak）。请点「重载配置」使改动生效。", p, filepath.Base(p)), nil
}
