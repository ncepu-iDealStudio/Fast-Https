// Package site 管理 include 目录下的站点文件（conf.d/<name>.json）。
// 站点文件与主配置 http.server[] 的元素同构，合并与校验都复用 config 包，
// 这里只负责命名、落盘原子性与目录扫描。
package site

import (
	"errors"
	"fast-https/config"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// namePattern 限制站点名就是文件名主干，避免生成 ../../etc/passwd 这类路径。
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Entry 是 site list 的一行。Listen 是多个监听的合并展示（"80, 443 ssl h2"）。
type Entry struct {
	Name       string
	File       string
	Listen     string
	ServerName string
	Types      []string
	Invalid    string // 文件解析或校验失败时的原因
	Enabled    bool
}

// ValidateName 检查站点名能否安全地当文件名用。
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New("站点名不能为空")
	case !namePattern.MatchString(name):
		return fmt.Errorf("站点名只能包含字母、数字、点、下划线与短横线：%q", name)
	case name == "." || name == "..":
		return fmt.Errorf("站点名不能是 %q", name)
	}
	return nil
}

// Path 返回站点名在给定目录下的文件路径。
func Path(dir, name string) string {
	return filepath.Join(dir, name+".json")
}

// DisabledPath 返回停用站点文件路径（<name>.json.disabled）。
func DisabledPath(dir, name string) string {
	return filepath.Join(dir, name+".json.disabled")
}

// Find 在多个 include 目录里找站点文件，返回第一个存在的路径。
func Find(dirs []string, name string) (string, bool) {
	for _, dir := range dirs {
		path := Path(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// FindDisabled 找停用的站点文件。
func FindDisabled(dirs []string, name string) (string, bool) {
	for _, dir := range dirs {
		path := DisabledPath(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// Create 写一个站点文件：先生成到 .tmp 并跑 config 的校验，通过后才 rename，
// 因此失败不会留下半个文件，也不会覆盖已有的合法站点。
func Create(dirs []string, name string, in config.SiteInput, force bool) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", errors.New("主配置没有可用的 http.include 目录，请先在 fast-https.json 里配置 http.include")
	}
	target := Path(dirs[0], name)
	if _, err := os.Stat(target); err == nil && !force {
		return "", fmt.Errorf("站点文件已存在：%s（要覆盖请加 --force）", target)
	}

	data, err := in.MarshalJSONFile()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create include dir: %w", err)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := config.ValidateNewSiteFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename %s: %w", tmp, err)
	}
	return target, nil
}

// LoadExisting 读取一个已存在的站点文件。不存在时返回 found=false 而不是错误：
// site init 靠它区分"新建"与"同名即更新"。
func LoadExisting(dirs []string, name string) (string, config.SiteInput, bool, error) {
	if strings.TrimSpace(name) == "" {
		return "", config.SiteInput{}, false, nil
	}
	path, ok := Find(dirs, name)
	if !ok {
		return "", config.SiteInput{}, false, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", config.SiteInput{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", config.SiteInput{}, false, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	return path, in, true, nil
}

// Update 就地改写一个已存在的站点文件：apply 只改它关心的字段。
//
// 写回不是"把结构体序列化回去"，而是把 apply 前后的差异翻译成 JSON 路径，
// 用 infra/jsonc 就地替换。因此文件里 site 这一层没有建模的字段
// （auth、proxy_cache、appfirewall、limit、zip、try、error_page……）
// 以及注释、缩进、字段顺序都原样保留。
func Update(dirs []string, name string, apply func(*config.SiteInput) error) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	path, ok := Find(dirs, name)
	if !ok {
		return "", fmt.Errorf("找不到站点文件：%s.json（新建请用 fast-https site init）", name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	// 解析两次：一份当 diff 基线，一份交给 apply。共享 slice 会让 apply 的
	// 就地修改同时改到基线，diff 就变成"什么都没变"。
	existing, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败：%w", path, err)
	}

	if err := apply(&in); err != nil {
		return "", err
	}

	data, err := config.PatchSiteFile(raw, existing, in)
	if err != nil {
		return "", fmt.Errorf("改写 %s 失败：%w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := config.ValidateNewSiteFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename %s: %w", tmp, err)
	}
	return path, nil
}

// Remove 删除站点文件。删前先确认它确实能解析成 server 块，
// 避免手滑删掉同名的其它文件（比如目录里的 README）。
func Remove(dirs []string, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	path, ok := Find(dirs, name)
	if !ok {
		path, ok = FindDisabled(dirs, name)
		if !ok {
			return "", fmt.Errorf("找不到站点文件：%s.json", name)
		}
	}
	if err := config.ValidateSiteFile(path); err != nil {
		// 校验失败也允许删除：用户可能正是想删掉一个写坏的文件。
		_ = err
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("remove %s: %w", path, err)
	}
	return path, nil
}

// List 扫描所有 include 目录，返回按文件名排序的站点摘要。
func List(dirs []string) ([]Entry, error) {
	var entries []Entry
	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read include dir %s: %w", dir, err)
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, file.Name())
			entry := Entry{Name: strings.TrimSuffix(file.Name(), ".json"), File: path, Enabled: true}

			raw, err := os.ReadFile(path)
			if err != nil {
				entry.Invalid = err.Error()
				entries = append(entries, entry)
				continue
			}
			in, err := config.ParseSiteInput(raw)
			if err != nil {
				entry.Invalid = err.Error()
				entries = append(entries, entry)
				continue
			}
			entry.Listen = in.ListenSummary()
			entry.ServerName = in.ServerName
			for _, location := range in.Locations {
				entry.Types = append(entry.Types, describeLocation(location))
			}
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// ListAll 列出启用与停用（*.json.disabled）的站点。
func ListAll(dirs []string) ([]Entry, error) {
	enabled, err := List(dirs)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, e := range enabled {
		seen[e.Name] = struct{}{}
	}
	var extra []Entry
	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read include dir %s: %w", dir, err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(strings.ToLower(file.Name()), ".json.disabled") {
				continue
			}
			name := file.Name()[:len(file.Name())-len(".json.disabled")]
			if _, ok := seen[name]; ok {
				continue
			}
			path := filepath.Join(dir, file.Name())
			entry := Entry{Name: name, File: path, Enabled: false}
			raw, err := os.ReadFile(path)
			if err != nil {
				entry.Invalid = err.Error()
				extra = append(extra, entry)
				continue
			}
			in, err := config.ParseSiteInput(raw)
			if err != nil {
				entry.Invalid = err.Error()
				extra = append(extra, entry)
				continue
			}
			entry.Listen = in.ListenSummary()
			entry.ServerName = in.ServerName
			for _, location := range in.Locations {
				entry.Types = append(entry.Types, describeLocation(location))
			}
			extra = append(extra, entry)
		}
	}
	entries := append(enabled, extra...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// Disable 把 <name>.json 改名为 <name>.json.disabled。
func Disable(dirs []string, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if _, ok := FindDisabled(dirs, name); ok {
		return "", fmt.Errorf("site %s is already disabled", name)
	}
	path, ok := Find(dirs, name)
	if !ok {
		return "", fmt.Errorf("找不到站点文件：%s.json", name)
	}
	dest := path + ".disabled"
	if err := os.Rename(path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// Enable 把 <name>.json.disabled 改回 <name>.json。
func Enable(dirs []string, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	if _, ok := Find(dirs, name); ok {
		return "", fmt.Errorf("site %s is already enabled", name)
	}
	path, ok := FindDisabled(dirs, name)
	if !ok {
		return "", fmt.Errorf("找不到停用的站点文件：%s.json.disabled", name)
	}
	dest := strings.TrimSuffix(path, ".disabled")
	if err := os.Rename(path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func describeLocation(location config.SiteLocation) string {
	url := location.URL
	if url == "" {
		url = "/"
	}
	kind := location.Type
	if kind == "" {
		kind = "local"
	}
	switch {
	case location.ProxyPass != "":
		return url + " → " + kind + " " + location.ProxyPass
	case location.Root != "":
		return url + " → " + kind + " " + location.Root
	default:
		return url + " → " + kind
	}
}
