package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SiteInput 是「添加 / 编辑站点」弹窗提交的字段。新站点写成 conf.d 下的独立
// JSON 文件，不改主配置，也不覆盖已有文件；编辑只回写表单覆盖的字段。
type SiteInput struct {
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`
	Index     string `json:"index"`
	Cert      string `json:"cert"`
	CertKey   string `json:"certKey"`
	FileName  string `json:"fileName"`
	Source    string `json:"source"`
}

// SiteDetail 是详情弹窗一次拉取的内容：表单字段 + 原始 JSON + 是否可编辑。
type SiteDetail struct {
	Source    string `json:"source"`
	Path      string `json:"path"`
	Editable  bool   `json:"editable"`
	Note      string `json:"note"`
	Name      string `json:"name"`
	Listen    string `json:"listen"`
	Type      string `json:"type"`
	Root      string `json:"root"`
	ProxyPass string `json:"proxyPass"`
	Index     string `json:"index"`
	Cert      string `json:"cert"`
	CertKey   string `json:"certKey"`
	Raw       string `json:"raw"`
}

type newSiteLocation struct {
	URL       string   `json:"url"`
	Type      string   `json:"type"`
	Root      string   `json:"root,omitempty"`
	Index     []string `json:"index,omitempty"`
	ProxyPass string   `json:"proxy_pass,omitempty"`
}

type newSite struct {
	Listen            []any             `json:"listen"`
	ServerName        string            `json:"server_name"`
	SSLCertificate    string            `json:"ssl_certificate,omitempty"`
	SSLCertificateKey string            `json:"ssl_certificate_key,omitempty"`
	Location          []newSiteLocation `json:"location"`
}

var siteFileNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// parsedSite 是 SiteInput 校验并归一化后的结果。
type parsedSite struct {
	name    string
	typ     string
	root    string
	pass    string
	listens []any
	ports   []string
	index   []string
	cert    string
	certKey string
	file    string
}

func parseSiteInput(in SiteInput) (parsedSite, error) {
	var p parsedSite
	p.name = strings.TrimSpace(in.Name)
	if p.name == "" {
		return p, fmt.Errorf("server_name 不能为空")
	}
	// 多域名支持：server_name 允许空格分隔多个域名（nginx 风格），
	// 每个域名都要非空且不含非法字符。
	names := strings.Fields(p.name)
	if len(names) == 0 {
		return p, fmt.Errorf("server_name 不能为空")
	}
	for _, name := range names {
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
		port, err := strconv.Atoi(fields[0])
		if err != nil || port < 1 || port > 65535 {
			return p, fmt.Errorf("监听项 %q 不是合法端口（1-65535）", raw)
		}
		p.ports = append(p.ports, ":"+strconv.Itoa(port))
		if len(fields) == 1 {
			p.listens = append(p.listens, port)
		} else {
			p.listens = append(p.listens, raw)
		}
	}
	if len(p.listens) == 0 {
		return p, fmt.Errorf("listen 至少需要一个监听端口")
	}
	for _, x := range strings.FieldsFunc(in.Index, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		if x = strings.TrimSpace(x); x != "" {
			p.index = append(p.index, x)
		}
	}
	p.cert = strings.TrimSpace(in.Cert)
	p.certKey = strings.TrimSpace(in.CertKey)
	if (p.cert == "") != (p.certKey == "") {
		return p, fmt.Errorf("证书路径与私钥路径必须同时填写或同时留空")
	}
	// listen 带 ssl 且证书留空时，用主程序自动生成的本地证书。
	// init.CertInit 按主域名生成 cert/<primary>.pem 与 <primary>-key.pem，
	// 这里写成同样的相对路径，reload 时就能直接找到。
	if p.cert == "" && strings.Contains(in.Listen, "ssl") {
		primary := strings.Fields(p.name)[0]
		p.cert = "./cert/" + primary + ".pem"
		p.certKey = "./cert/" + primary + "-key.pem"
	}
	return p, nil
}

func addSiteFile(dir string, in SiteInput) (string, error) {
	p, err := parseSiteInput(in)
	if err != nil {
		return "", err
	}

	file := strings.TrimSpace(in.FileName)
	if file == "" {
		// 多域名：文件名用第一个域名（主域名）。
		file = sanitizeSiteName(strings.Fields(p.name)[0])
	}
	file = strings.TrimSuffix(file, ".json")
	if !siteFileNameRe.MatchString(file) {
		return "", fmt.Errorf("文件名只能包含字母、数字、点、下划线与连字符，且不能以点开头")
	}

	etcDir := filepath.Dir(resolvePaths(dir).Config)
	target := filepath.Join(etcDir, "conf.d", file+".json")
	// 同名文件先报，比端口冲突更直接，也不会覆盖用户已有配置。
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("站点文件已存在：conf.d/%s.json，请换个文件名或先删除旧文件", file)
	}

	// 同一端口挂不同 server_name 是合法的虚拟主机写法，只有同名又同端口
	// 才是真冲突；这里在写文件前拦下，避免 reload 时才报错。
	// 多域名：逐个域名检查是否与已有站点冲突。
	if sites, _, err := readSites(dir); err == nil {
		newNames := strings.Fields(p.name)
		for _, s := range sites {
			existingNames := strings.Fields(s.Name)
			for _, nn := range newNames {
				for _, en := range existingNames {
					if nn != en {
						continue
					}
					for _, port := range p.ports {
						if s.Port == port {
							return "", fmt.Errorf("域名 %s 已在站点 %s 监听 %s 上使用，换个端口或改个名称", nn, s.Name, port)
						}
					}
				}
			}
		}
	}

	loc := newSiteLocation{URL: "/", Type: p.typ}
	if p.typ == "local" {
		loc.Root = p.root
		loc.Index = p.index
		if len(loc.Index) == 0 {
			loc.Index = []string{"index.html", "index.htm"}
		}
	} else {
		loc.ProxyPass = p.pass
	}
	site := newSite{
		Listen:            p.listens,
		ServerName:        p.name,
		SSLCertificate:    p.cert,
		SSLCertificateKey: p.certKey,
		Location:          []newSiteLocation{loc},
	}
	data, err := json.MarshalIndent(site, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeNewFileAtomic(target, append(data, '\n')); err != nil {
		return "", err
	}
	return fmt.Sprintf("已创建 conf.d/%s.json（%s，监听 %s）。请点「重载配置」使站点生效。", file, p.name, strings.Join(p.ports, " ")), nil
}

// deleteSiteFile 删除 conf.d 下的站点文件。source 必须是 readSites 给出的相对
// 路径（conf.d/xxx.json）；主配置 http.server[] 内联的站点没有独立文件，不可删。
// 删除用 rename 成 .bak 而不是直接 unlink：conf.d 的 include 只收集 *.json，
// .bak 不会被加载，误删后可手工找回。
func deleteSiteFile(dir, source string) (string, error) {
	paths := resolvePaths(dir)
	etcDir := filepath.Dir(paths.Config)
	rel := filepath.ToSlash(strings.TrimSpace(source))
	if rel == "" {
		return "", fmt.Errorf("缺少站点来源")
	}
	if filepath.Base(paths.Config) == rel {
		return "", fmt.Errorf("该站点内联在主配置 http.server[] 里，没有独立文件可删")
	}
	if !strings.HasPrefix(rel, "conf.d/") || !strings.HasSuffix(rel, ".json") {
		return "", fmt.Errorf("只允许删除 conf.d 下的站点文件：%s", rel)
	}
	full := filepath.Join(etcDir, filepath.FromSlash(rel))
	if !withinDir(etcDir, full) {
		return "", fmt.Errorf("站点路径不合法：%s", source)
	}
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("站点文件不存在：%s", rel)
	}
	backup := full + ".bak"
	if _, err := os.Stat(backup); err == nil {
		if err := os.Remove(backup); err != nil {
			return "", fmt.Errorf("清理旧备份失败：%w", err)
		}
	}
	if err := os.Rename(full, backup); err != nil {
		return "", fmt.Errorf("删除失败：%w", err)
	}
	return fmt.Sprintf("已删除 %s（备份为 %s.bak）。请点「重载配置」使删除生效。", rel, rel), nil
}

// readSiteDetail 读取站点文件的表单字段与原文。主配置 http.server[] 里的内联
// 站点不在这里编辑，返回 editable=false 加提示。
func readSiteDetail(dir, source string) (SiteDetail, error) {
	paths := resolvePaths(dir)
	etcDir := filepath.Dir(paths.Config)
	rel := filepath.ToSlash(strings.TrimSpace(source))
	if rel == "" {
		return SiteDetail{}, fmt.Errorf("缺少站点来源")
	}
	if filepath.Base(paths.Config) == rel {
		return SiteDetail{
			Source: rel, Path: paths.Config,
			Note: "该站点内联在主配置 http.server[] 里，请到设置页的主配置表单里修改",
		}, nil
	}
	full := filepath.Join(etcDir, filepath.FromSlash(rel))
	if !withinDir(etcDir, full) {
		return SiteDetail{}, fmt.Errorf("站点路径不合法：%s", source)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return SiteDetail{}, fmt.Errorf("读不到站点文件 %s：%w", rel, err)
	}
	var m map[string]any
	if err := json.Unmarshal(stripJSONC(raw), &m); err != nil {
		return SiteDetail{}, fmt.Errorf("站点文件不是合法 JSON：%w", err)
	}
	d := SiteDetail{Source: rel, Path: full, Editable: true, Raw: string(raw)}
	d.Name, _ = m["server_name"].(string)
	d.Cert, _ = m["ssl_certificate"].(string)
	d.CertKey, _ = m["ssl_certificate_key"].(string)
	d.Listen = formatListenValue(m["listen"])
	if arr, ok := m["location"].([]any); ok && len(arr) > 0 {
		if loc, ok := arr[0].(map[string]any); ok {
			d.Type, _ = loc["type"].(string)
			d.Root, _ = loc["root"].(string)
			d.ProxyPass, _ = loc["proxy_pass"].(string)
			if idx, ok := loc["index"].([]any); ok {
				items := make([]string, 0, len(idx))
				for _, x := range idx {
					items = append(items, fmt.Sprint(x))
				}
				d.Index = strings.Join(items, " ")
			}
		}
	}
	if d.Type == "" {
		d.Type = "local"
	}
	return d, nil
}

// saveSiteDetail 只回写表单覆盖的字段，location 里未涉及的键（rewrite、auth、
// proxy_set_header 等）与其余 location 原样保留。
func saveSiteDetail(dir string, in SiteInput) (string, error) {
	p, err := parseSiteInput(in)
	if err != nil {
		return "", err
	}
	paths := resolvePaths(dir)
	etcDir := filepath.Dir(paths.Config)
	rel := filepath.ToSlash(strings.TrimSpace(in.Source))
	if rel == "" {
		return "", fmt.Errorf("缺少站点来源")
	}
	if filepath.Base(paths.Config) == rel {
		return "", fmt.Errorf("主配置里的内联站点请在设置页修改")
	}
	full := filepath.Join(etcDir, filepath.FromSlash(rel))
	if !withinDir(etcDir, full) {
		return "", fmt.Errorf("站点路径不合法：%s", in.Source)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return "", fmt.Errorf("读不到站点文件 %s：%w", rel, err)
	}
	var m map[string]any
	if err := json.Unmarshal(stripJSONC(raw), &m); err != nil {
		return "", fmt.Errorf("站点文件不是合法 JSON：%w", err)
	}

	m["server_name"] = p.name
	m["listen"] = p.listens
	setOptionalString(m, "ssl_certificate", p.cert)
	setOptionalString(m, "ssl_certificate_key", p.certKey)

	locs, _ := m["location"].([]any)
	var loc map[string]any
	if len(locs) > 0 {
		loc, _ = locs[0].(map[string]any)
	}
	if loc == nil {
		loc = map[string]any{"url": "/"}
		locs = []any{loc}
		m["location"] = locs
	}
	if _, ok := loc["url"]; !ok {
		loc["url"] = "/"
	}
	loc["type"] = p.typ
	if p.typ == "local" {
		loc["root"] = p.root
		delete(loc, "proxy_pass")
		if len(p.index) > 0 {
			loc["index"] = p.index
		} else {
			delete(loc, "index")
		}
	} else {
		loc["proxy_pass"] = p.pass
		delete(loc, "root")
		delete(loc, "index")
	}

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return saveConfigFile(full, string(out)+"\n")
}

func setOptionalString(m map[string]any, key, value string) {
	if value == "" {
		delete(m, key)
		return
	}
	m[key] = value
}

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

func withinDir(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// PickPath 调系统原生选择框，返回相对 config 目录的路径。
// kind 为 "dir" 时选目录（站点根目录），否则选文件（证书 / 私钥）。
// 用户取消时返回空字符串，前端保持输入框不变。
func (s *DesktopService) PickPath(kind string) (string, error) {
	if appRef == nil {
		return "", fmt.Errorf("应用尚未就绪")
	}
	etcDir := filepath.Dir(resolvePaths(s.baseDir).Config)
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
	return relativeToEtc(etcDir, p), nil
}

// relativeToEtc 把绝对路径转成相对 config 目录的写法（./cert/a.pem），
// 与手写配置保持一致；不在 config 内的路径保留绝对路径。
func relativeToEtc(etcDir, p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(etcDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(abs)
	}
	return "./" + filepath.ToSlash(rel)
}

func sanitizeSiteName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		out = "site"
	}
	return out
}

// writeNewFileAtomic 新建站点文件：同目录临时文件 + rename，避免写一半留下坏 JSON。
func writeNewFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".site-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0644); err != nil {
		return err
	}
	return os.Rename(name, path)
}
