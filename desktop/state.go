package main

// 桌面端是独立进程，不 import 主项目的 config 包：
// 主配置的 schema 结构体未导出，且路径解析依赖编译期 build tag 与进程 CWD，
// 直接复用会把服务器的工作目录语义带进桌面端。这里改为「读文件 + 调 CLI」：
// 路径基址由 `fast-https config path --json` 给出，配置与证书自己解析。

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const desktopVersion = "1.6.0"

// RuntimePaths 与 cmd/config_path.go 的 RuntimePaths 一致。
type RuntimePaths struct {
	Config string `json:"config"`
	Cert   string `json:"cert"`
	Log    string `json:"log"`
	Pid    string `json:"pid"`
}

// Overview 是概览页一次拉取的全部内容。
type Overview struct {
	Running         bool         `json:"running"`
	PID             int          `json:"pid"`
	Version         string       `json:"version"`
	Ports           []string     `json:"ports"`
	SiteCount       int          `json:"siteCount"`
	TLSCount        int          `json:"tlsCount"`
	NearestCertDays int          `json:"nearestCertDays"`
	ConfigDirty     bool         `json:"configDirty"`
	Paths           RuntimePaths `json:"paths"`
	CheckedAt       string       `json:"checkedAt"`
	Error           string       `json:"error"`
}

// Site 是一个 server 块，可能来自主配置 http.server[] 或 conf.d/*.json。
type Site struct {
	Name     string `json:"name"`
	Port     string `json:"port"`
	Listen   string `json:"listen"`
	TLS      bool   `json:"tls"`
	Root     string `json:"root"`
	CertFile string `json:"certFile"`
	Source   string `json:"source"`
}

// Cert 是从 PEM 里读出的证书摘要。
type Cert struct {
	Domain     string `json:"domain"`
	Issuer     string `json:"issuer"`
	NotAfter   string `json:"notAfter"`
	DaysLeft   int    `json:"daysLeft"`
	SelfSigned bool   `json:"selfSigned"`
	Note       string `json:"note"`
}

type mainConfig struct {
	LogRoot string `json:"log_root"`
	HTTP    struct {
		Include []string   `json:"include"`
		Servers []siteFile `json:"server"`
	} `json:"http"`
}

// siteFile 与 config/schema.go 的 jsonServer 形状对齐，但只取界面要展示的字段。
type siteFile struct {
	Listen         json.RawMessage `json:"listen"`
	ServerName     string          `json:"server_name"`
	SSLCertificate string          `json:"ssl_certificate"`
	Locations      []struct {
		Root string `json:"root"`
	} `json:"location"`
}

func exeDir() string {
	e, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(e)
}

func serverBinary(dir string) string {
	name := "fast-https"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// resolvePaths 优先让主程序自己报路径（归档 / deb / rpm / apk 布局不同），
// 报不出来再按归档布局兜底。
func resolvePaths(dir string) RuntimePaths {
	exe := serverBinary(dir)
	if _, err := os.Stat(exe); err == nil {
		cmd := exec.Command(exe, "config", "path", "--json")
		cmd.Dir = dir
		if out, err := cmd.Output(); err == nil {
			var p RuntimePaths
			if json.Unmarshal(out, &p) == nil && p.Config != "" {
				return p
			}
		}
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
		Config: filepath.Join(base, "config", "fast-https.json"),
		Cert:   filepath.Join(base, "config", "cert"),
		Log:    filepath.Join(base, "logs"),
		Pid:    filepath.Join(base, "fast-https.pid"),
	}
}

// runServerCommand 把操作交给主程序，桌面端不自己实现进程管理。
func runServerCommand(dir string, args ...string) (string, error) {
	exe := serverBinary(dir)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("未找到主程序 %s：桌面端需要与主程序放在同一目录", exe)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return "", fmt.Errorf("执行 %s %s 失败：%w", filepath.Base(exe), strings.Join(args, " "), err)
	}
	return text, nil
}

// startServer 用 Start 而非 Run：start 子命令会派生后台进程，
// 桌面端不能等它退出，但要 Wait 回收，避免留下僵尸进程。
func startServer(dir string) (string, error) {
	exe := serverBinary(dir)
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("未找到主程序 %s", exe)
	}
	cmd := exec.Command(exe, "start")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动失败：%w", err)
	}
	go func() { _ = cmd.Wait() }()
	return "启动指令已发送，状态会在 2 秒后刷新", nil
}

// stripJSONC 去掉 // 与 /* */ 注释以及尾逗号。
// 主配置与站点文件都允许 JSONC，标准库的 json 不接受这些写法。
func stripJSONC(b []byte) []byte {
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

// parseListen 兼容主配置的三种写法：80 / "443 ssl h2" / [80, "443 ssl h2"]。
func parseListen(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var one interface{}
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil
	}
	switch v := one.(type) {
	case float64:
		return []string{strconv.Itoa(int(v))}
	case string:
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, e := range v {
			switch x := e.(type) {
			case float64:
				out = append(out, strconv.Itoa(int(x)))
			case string:
				out = append(out, x)
			}
		}
		return out
	}
	return nil
}

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

// etcPath 复刻主程序 absETCPath 的常见情形：相对路径以主配置所在目录为基址。
// 旧配置里还会出现 "./config/xxx" 这种带前缀的写法，一并兼容。
func etcPath(etcDir, p string) string {
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

func collectJSONFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".json") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func readSites(dir string) ([]Site, RuntimePaths, error) {
	paths := resolvePaths(dir)
	raw, err := os.ReadFile(paths.Config)
	if err != nil {
		return nil, paths, fmt.Errorf("读不到主配置 %s：%w", paths.Config, err)
	}
	var cfg mainConfig
	if err := json.Unmarshal(stripJSONC(raw), &cfg); err != nil {
		return nil, paths, fmt.Errorf("主配置不是合法 JSON：%w", err)
	}
	etcDir := filepath.Dir(paths.Config)

	var sources []struct {
		file string
		body siteFile
	}
	for _, s := range cfg.HTTP.Servers {
		sources = append(sources, struct {
			file string
			body siteFile
		}{file: filepath.Base(paths.Config), body: s})
	}
	for _, inc := range cfg.HTTP.Include {
		target := etcPath(etcDir, inc)
		info, err := os.Stat(target)
		if err != nil {
			continue
		}
		var list []string
		if info.IsDir() {
			list = collectJSONFiles(target)
		} else if strings.EqualFold(filepath.Ext(target), ".json") {
			list = []string{target}
		}
		for _, f := range list {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var s siteFile
			if json.Unmarshal(stripJSONC(b), &s) != nil {
				continue
			}
			rel, _ := filepath.Rel(etcDir, f)
			sources = append(sources, struct {
				file string
				body siteFile
			}{file: rel, body: s})
		}
	}

	var sites []Site
	for _, src := range sources {
		s := src.body
		listens := parseListen(s.Listen)
		if len(listens) == 0 {
			listens = []string{""}
		}
		root := ""
		for _, l := range s.Locations {
			if l.Root != "" {
				root = etcPath(etcDir, l.Root)
				break
			}
		}
		for _, listen := range listens {
			name := s.ServerName
			if name == "" {
				name = "(未命名站点)"
			}
			sites = append(sites, Site{
				Name:     name,
				Port:     portOf(listen),
				Listen:   listen,
				TLS:      strings.Contains(listen, "ssl"),
				Root:     root,
				CertFile: etcPath(etcDir, s.SSLCertificate),
				Source:   src.file,
			})
		}
	}
	return sites, paths, nil
}

func readCerts(dir string) ([]Cert, error) {
	sites, _, err := readSites(dir)
	if err != nil {
		return nil, err
	}
	var certs []Cert
	seen := map[string]bool{}
	for _, s := range sites {
		if !s.TLS || s.CertFile == "" || seen[s.CertFile] {
			continue
		}
		seen[s.CertFile] = true
		certs = append(certs, inspectPEM(s.Name, s.CertFile))
	}
	return certs, nil
}

func inspectPEM(domain, path string) Cert {
	c := Cert{Domain: domain, Note: path}
	b, err := os.ReadFile(path)
	if err != nil {
		c.Note = "证书文件不存在：" + path
		return c
	}
	for len(b) > 0 {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			c.Note = "证书解析失败：" + err.Error()
			return c
		}
		c.Issuer = cert.Issuer.CommonName
		if c.Issuer == "" {
			c.Issuer = cert.Issuer.String()
		}
		c.NotAfter = cert.NotAfter.Format("2006-01-02")
		c.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
		c.SelfSigned = cert.Issuer.String() == cert.Subject.String()
		c.Note = path
		return c
	}
	c.Note = "文件里没有 CERTIFICATE 块：" + path
	return c
}

// probePorts 判断服务在不在跑：对配置里声明的端口做一次回环 TCP 连接。
// 比读 pid 文件可靠，也避开 Windows / Unix 进程判活的平台差异。
func probePorts(ports []string) bool {
	for _, p := range ports {
		if p == "" {
			continue
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1"+p, 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func readPID(pidFile string) int {
	b, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	var m map[string]int
	if json.Unmarshal(b, &m) != nil {
		return 0
	}
	return m["pid"]
}

// configDirty 是启发式判断：配置或站点文件比 pid 文件更新，
// 就认为「已改未重载」。pid 是启动/重载时写的，改动配置不会动它。
func configDirty(paths RuntimePaths, sites []Site) bool {
	pidInfo, err := os.Stat(paths.Pid)
	if err != nil {
		return false
	}
	files := []string{paths.Config}
	if entries := collectJSONFiles(filepath.Dir(paths.Config)); len(entries) > 0 {
		files = append(files, entries...)
	}
	for _, s := range sites {
		if s.Source != "" && !filepath.IsAbs(s.Source) {
			files = append(files, etcPath(filepath.Dir(paths.Config), s.Source))
		}
	}
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(pidInfo.ModTime()) {
			return true
		}
	}
	return false
}

func collectOverview(dir string) Overview {
	ov := Overview{
		Version:   desktopVersion,
		CheckedAt: time.Now().Format("15:04:05"),
	}
	sites, paths, err := readSites(dir)
	ov.Paths = paths
	if err != nil {
		ov.Error = err.Error()
		return ov
	}
	ov.SiteCount = len(sites)
	seenPort := map[string]bool{}
	for _, s := range sites {
		if s.TLS {
			ov.TLSCount++
		}
		if s.Port != "" && !seenPort[s.Port] {
			seenPort[s.Port] = true
			ov.Ports = append(ov.Ports, s.Port)
		}
	}
	sort.Strings(ov.Ports)
	ov.Running = probePorts(ov.Ports)
	ov.PID = readPID(paths.Pid)
	if out, err := runServerCommand(dir, "version"); err == nil && out != "" {
		ov.Version = strings.TrimSpace(strings.Split(out, "\n")[0])
	}
	if certs, err := readCerts(dir); err == nil {
		for _, c := range certs {
			if c.DaysLeft > 0 && (ov.NearestCertDays == 0 || c.DaysLeft < ov.NearestCertDays) {
				ov.NearestCertDays = c.DaysLeft
			}
		}
	}
	ov.ConfigDirty = configDirty(paths, sites)
	return ov
}

// tailLog 只读文件尾部，避免 access.log 很大时整文件进内存。
func tailLog(logDir, name string, n int) ([]string, error) {
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
	f, err := os.Open(filepath.Join(logDir, name+".log"))
	if err != nil {
		return nil, fmt.Errorf("读不到 %s.log：%w", name, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const maxRead = 256 * 1024
	start := int64(0)
	if st.Size() > maxRead {
		start = st.Size() - maxRead
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.TrimRight(l, "\r"))
	}
	return out, nil
}

// openPath 用系统默认方式打开文件或目录。
func openPath(path string) (string, error) {
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
