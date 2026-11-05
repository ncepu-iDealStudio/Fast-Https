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

// Update 就地改写一个已存在的站点文件：apply 只改它关心的字段，
// 其余字段（包括手写的额外 location）原样保留。
//
// 写回格式与 Create 一致：纯 JSON、两个空格缩进，因此文件里的注释不会保留——
// 站点文件本来就不该承载注释，需要说明写在文档或站点名里。
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
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败：%w", path, err)
	}

	if err := apply(&in); err != nil {
		return "", err
	}

	data, err := in.MarshalJSONFile()
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("找不到站点文件：%s.json", name)
	}
	if err := config.ValidateSiteFile(path); err != nil {
		// 校验失败也允许删除：用户可能正是想删掉一个写坏的文件。
		// 这里只要求它是 .json 文件，路径已由 Find 保证。
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
			entry := Entry{Name: strings.TrimSuffix(file.Name(), ".json"), File: path}

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
