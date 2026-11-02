package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 站点文件（config/conf.d/<name>.json）与 http.server[] 的元素同构，
// 合并后就是主配置里的一个 server 块。这里只暴露 CLI 需要编辑的子集：
// 没列出的字段（rewrite、try、proxy_cache…）由用户手写，读写时原样保留。

// SiteLocation 是站点文件里一个 location 的可编辑子集。
type SiteLocation struct {
	URL       string   `json:"url"`
	Type      string   `json:"type"`
	Root      string   `json:"root,omitempty"`
	Index     []string `json:"index,omitempty"`
	ProxyPass string   `json:"proxy_pass,omitempty"`
}

// SiteInput 是生成 conf.d/<name>.json 的输入。
// Listens 可以有多条（例如 ["80", "443 ssl h2"]），站点文件里就写成数组。
type SiteInput struct {
	Listens           []string
	ServerName        string
	SSLCertificate    string
	SSLCertificateKey string
	Locations         []SiteLocation
}

// ListenSummary 把监听列表合成一行，供 CLI 输出与 site list 使用。
func (in SiteInput) ListenSummary() string {
	return strings.Join(normalizeListens(in.Listens), ", ")
}

// siteFileJSON 是落盘格式。listen 允许数字、字符串或数组（与 jsonServer 的
// flexList 一致）：纯数字写成数字，带 "ssl"/"h2" 时写成字符串。
type siteFileJSON struct {
	Listen            flexList       `json:"listen"`
	ServerName        string         `json:"server_name"`
	SSLCertificate    string         `json:"ssl_certificate,omitempty"`
	SSLCertificateKey string         `json:"ssl_certificate_key,omitempty"`
	Locations         []SiteLocation `json:"location"`
}

// MarshalJSONFile 生成站点文件内容，缩进与主配置一致（两个空格）。
// 单个监听写成标量，多个写成数组，两种写法加载时等价。
func (in SiteInput) MarshalJSONFile() ([]byte, error) {
	listens := normalizeListens(in.Listens)
	if len(listens) == 0 {
		listens = []string{"80"}
	}
	payload := siteFileJSON{
		Listen:            flexList(listens),
		ServerName:        strings.TrimSpace(in.ServerName),
		SSLCertificate:    strings.TrimSpace(in.SSLCertificate),
		SSLCertificateKey: strings.TrimSpace(in.SSLCertificateKey),
		Locations:         in.Locations,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal site file: %w", err)
	}
	return append(data, '\n'), nil
}

// normalizeListens 去掉空白与空项，保持书写顺序（顺序即 site list 的展示顺序）。
func normalizeListens(listens []string) []string {
	out := make([]string, 0, len(listens))
	for _, listen := range listens {
		if value := strings.TrimSpace(listen); value != "" {
			out = append(out, value)
		}
	}
	return out
}

// ParseSiteInput 读取站点文件的可编辑子集。未列出的字段不会出现在结果里，
// 因此这个函数只用于摘要与向导预填，不能拿来整体回写。
func ParseSiteInput(raw []byte) (SiteInput, error) {
	var payload siteFileJSON
	if err := unmarshalConfig(raw, &payload); err != nil {
		return SiteInput{}, err
	}
	in := SiteInput{
		Listens:           normalizeListens(payload.Listen),
		ServerName:        payload.ServerName,
		SSLCertificate:    payload.SSLCertificate,
		SSLCertificateKey: payload.SSLCertificateKey,
		Locations:         payload.Locations,
	}
	return in, nil
}

// ValidateSiteFile 用主配置同一套 schema 校验单个站点文件，
// 报错里带文件路径与字段路径（例如 conf.d/a.json: location[0].proxy_pass ...）。
func ValidateSiteFile(path string) error {
	return validateSiteFile(path, true)
}

// ValidateNewSiteFile 与 ValidateSiteFile 相同，但允许 TLS 证书文件还不存在：
// site init 生成的默认证书路径要等首次 start 才由本地 CA 落盘。
func ValidateNewSiteFile(path string) error {
	return validateSiteFile(path, false)
}

func validateSiteFile(path string, requireCerts bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read site file failed: %w", err)
	}
	var server jsonServer
	if err := unmarshalConfig(content, &server); err != nil {
		return fmt.Errorf("parse site json failed (%s): %w", path, err)
	}
	baseDir := filepath.Dir(path)
	// include 文件里的相对路径以文件所在目录为基准，rootBaseDir 用主配置目录，
	// 这里没有主配置上下文，两者都取文件目录。
	return validateServer(&server, baseDir, baseDir, path, requireCerts)
}

// SiteDirs 返回运行期快照里 include 目录的绝对路径。
func SiteDirs(cfg *Fast_Https) []string {
	if cfg == nil {
		return nil
	}
	return siteDirsFromInclude(cfg.Include)
}

// SiteDirsFromFile 从指定配置文件读取 include 目录，不替换运行期快照。
// site 命令的 --config 走它，才能既支持自定义路径又不污染进程状态。
func SiteDirsFromFile(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}
	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return nil, fmt.Errorf("parse config json failed: %w", err)
	}
	return siteDirsFromInclude(root.HTTP.Include), nil
}

// siteDirsFromInclude 把 include 配置转成存在的目录绝对路径列表。
func siteDirsFromInclude(includes []string) []string {
	var dirs []string
	seen := make(map[string]struct{}, len(includes))
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			continue
		}
		fullPath := includePath
		if !filepath.IsAbs(fullPath) {
			if _, err := os.Stat(fullPath); err != nil {
				fullPath = absConfigPath(includePath)
			}
		}
		info, err := os.Stat(fullPath)
		if err != nil || !info.IsDir() {
			continue
		}
		abs, err := filepath.Abs(fullPath)
		if err != nil {
			abs = fullPath
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		dirs = append(dirs, abs)
	}
	sort.Strings(dirs)
	return dirs
}

// SnapshotServer 按 server_name 在"主配置 + include"合并视图里找 server 块，
// 返回该块的 JSON 与来源文件。直接序列化 schema 对象，不做运行期对象反推，
// 因此字段与配置文件逐字一致。
func SnapshotServer(mainPath, name string) ([]byte, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("site name is required")
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		return nil, "", fmt.Errorf("read config file failed: %w", err)
	}
	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return nil, "", fmt.Errorf("parse config json failed: %w", err)
	}

	dirs := siteDirsFromInclude(root.HTTP.Include)

	// 先按站点名（文件名主干）找：site show 的参数就是 site init 用的名字，
	// 与 server_name 不一定相同。
	for _, dir := range dirs {
		path := filepath.Join(dir, name+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var server jsonServer
		if err := unmarshalConfig(raw, &server); err != nil {
			return nil, "", fmt.Errorf("parse %s: %w", path, err)
		}
		data, err := marshalServerSpec(&server)
		return data, path, err
	}

	for i := range root.HTTP.Servers {
		if root.HTTP.Servers[i].ServerName == name {
			data, err := marshalServerSpec(&root.HTTP.Servers[i])
			return data, mainPath, err
		}
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var server jsonServer
			if err := unmarshalConfig(raw, &server); err != nil {
				continue
			}
			if server.ServerName == name {
				data, err := marshalServerSpec(&server)
				return data, path, err
			}
		}
	}
	return nil, "", fmt.Errorf("no server block with server_name %q", name)
}

func marshalServerSpec(server *jsonServer) ([]byte, error) {
	data, err := json.MarshalIndent(server, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal server block: %w", err)
	}
	return append(data, '\n'), nil
}


