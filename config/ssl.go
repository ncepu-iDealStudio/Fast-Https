package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SSLDecl 是配置里「一个域名使用哪张证书」的声明，按运行期的规则展开：
// server_name 的第一个域名用 ssl_certificate，其余域名用
// <证书目录>/<域名>.pem（与 runtime/listener 一致）。
type SSLDecl struct {
	Site   string // 站点名：include 文件的主干名；主配置内联的 server 块记为 "main"
	Source string // 来源文件的绝对路径
	Domain string // server_name 里的一个域名，保持书写形式
	// Listens 是该 server 块里带 ssl 的 listen 写法。
	Listens []string

	Certificate       string // 证书文件路径；找不到时是运行期会去读的那个路径
	CertificateKey    string
	CertificateExists bool
	KeyExists         bool
}

// SSLServersFromFile 读取配置文件（含 http.include 站点）里所有带 ssl 监听的 server 块，
// 返回按域名展开的证书声明，以及 acme 段（补过默认值，没有 acme 段时为 nil）。
// 只读：不替换运行期快照，也不写日志。
func SSLServersFromFile(path string) ([]SSLDecl, *AcmeConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config file failed: %w", err)
	}
	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return nil, nil, fmt.Errorf("parse config json failed: %w", err)
	}

	absMain, err := filepath.Abs(path)
	if err != nil {
		absMain = path
	}
	mainDir := filepath.Dir(absMain)

	var decls []SSLDecl
	for i := range root.HTTP.Servers {
		decls = append(decls, sslDecls("main", absMain, mainDir, mainDir, &root.HTTP.Servers[i])...)
	}

	files, err := includeFiles(root.HTTP.Include)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, nil, fmt.Errorf("read include file failed: %w", err)
		}
		var js jsonServer
		if err := unmarshalConfig(raw, &js); err != nil {
			return nil, nil, fmt.Errorf("parse include file %s failed: %w", file, err)
		}
		site := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		decls = append(decls, sslDecls(site, file, filepath.Dir(file), mainDir, &js)...)
	}

	acme := root.Acme
	acme.applyDefaults()
	return decls, acme, nil
}

func sslDecls(site, source, fileDir, mainDir string, js *jsonServer) []SSLDecl {
	var listens []string
	for _, l := range js.Listen {
		if strings.Contains(l, "ssl") {
			listens = append(listens, l)
		}
	}
	if len(listens) == 0 {
		return nil
	}

	var out []SSLDecl
	for i, name := range strings.Fields(js.ServerName) {
		d := SSLDecl{Site: site, Source: source, Domain: name, Listens: listens}
		if i == 0 {
			d.Certificate, d.CertificateExists = resolveDeclaredPath(js.SSLCertificate, fileDir, mainDir)
			d.CertificateKey, d.KeyExists = resolveDeclaredPath(js.SSLCertificateKey, fileDir, mainDir)
		} else {
			d.Certificate, d.CertificateExists = aliasCertPath(name, ".pem")
			d.CertificateKey, d.KeyExists = aliasCertPath(name, "-key.pem")
		}
		out = append(out, d)
	}
	return out
}

// resolveDeclaredPath 按 validateServer 同一套候选顺序找证书文件：找到返回绝对路径与 true；
// 找不到返回运行期会去读的路径（按 ETC 基址解析）与 false。空路径返回空串。
func resolveDeclaredPath(p, fileDir, mainDir string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	candidates := []string{p, absETCPath(p), absDataPath(p)}
	if !filepath.IsAbs(p) {
		candidates = append(candidates, filepath.Join(fileDir, p), filepath.Join(mainDir, p))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return toAbsPath(c), true
		}
	}
	return toAbsPath(absETCPath(p)), false
}

func aliasCertPath(domain, suffix string) (string, bool) {
	p := toAbsPath(filepath.Join(CERT_DIR, domain) + suffix)
	_, err := os.Stat(p)
	return p, err == nil
}

func toAbsPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
