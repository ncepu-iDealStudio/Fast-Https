// Package cacheconf 读写主配置顶层 cache 段：缓存区（目录、容量、有效期）
// 只在这里定义一次，站点文件用 proxy_cache 引用区名。
//
// 与 proxyconf/tlsconf 同样的做法：文本级局部替换，注释、缩进、字段顺序
// 原样保留；写入前回读校验。缓存区清空时删除整个 cache 成员。
package cacheconf

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"fast-https/config/internal/model"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/sitefile"
	"fast-https/infra/files"
	"fast-https/infra/jsonc"
)

// ReadCacheZones 读取主配置里的缓存区；没有 cache 段时返回空列表。
func ReadCacheZones(path string) ([]model.CacheZone, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseZones(raw, path)
}

func parseZones(raw []byte, path string) ([]model.CacheZone, error) {
	var probe struct {
		Cache []model.CacheZone `json:"cache"`
	}
	if err := schema.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if probe.Cache == nil {
		return []model.CacheZone{}, nil
	}
	return probe.Cache, nil
}

// PreviewCacheZones 返回写入 cache 段之后的完整文件内容，不落盘。
// 空列表等价于「没有缓存区」，此时删除整个 cache 成员。
func PreviewCacheZones(path string, zones []model.CacheZone) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var patched []byte
	if len(zones) == 0 {
		patched, err = jsonc.PatchPath(raw, []jsonc.Edit{{
			Path: []jsonc.Step{jsonc.Key("cache")},
		}})
	} else {
		patched, err = jsonc.PatchMember(raw, "cache", func(indent string) ([]byte, error) {
			return marshalZones(zones, indent)
		}, jsonc.PatchOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := verifyZones(patched, zones, path); err != nil {
		return nil, fmt.Errorf("%s: refused to write, result does not parse back: %w", path, err)
	}
	return patched, nil
}

// WriteCacheZones 把缓存区列表写进配置文件：已有 cache 键只替换值，
// 没有则追加；空列表删除该键。写入用临时文件 + rename。
func WriteCacheZones(path string, zones []model.CacheZone) error {
	patched, err := PreviewCacheZones(path, zones)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if stat, err := os.Stat(path); err == nil {
		mode = stat.Mode().Perm()
	}
	return files.WriteAtomic(path, patched, mode)
}

// CacheZoneRefs 返回引用该缓存区的位置描述，用于删除前检查。
// 范围包含主配置 http.server[] 与 http.include 目录下的站点文件（含停用）。
func CacheZoneRefs(mainPath, zone string) ([]string, error) {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(mainPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mainPath, err)
	}
	var root schema.Config
	if err := schema.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", mainPath, err)
	}

	var refs []string
	for i := range root.HTTP.Servers {
		refs = append(refs, serverRefs(mainPath, fmt.Sprintf("http.server[%d]", i), &root.HTTP.Servers[i], zone)...)
	}
	dirs, err := sitefile.SiteDirsFromFile(mainPath)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := strings.ToLower(entry.Name())
			if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".json.disabled") {
				continue
			}
			file := dir + string(os.PathSeparator) + entry.Name()
			body, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			var server schema.Server
			if err := schema.Unmarshal(body, &server); err != nil {
				// 解析失败的站点由校验单独报错；这里只做引用检查。
				continue
			}
			refs = append(refs, serverRefs(file, "", &server, zone)...)
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func serverRefs(file, where string, server *schema.Server, zone string) []string {
	var refs []string
	prefix := strings.TrimSpace(file + " " + where)
	for i := range server.Locations {
		loc := &server.Locations[i]
		if strings.TrimSpace(string(loc.ProxyCache)) != zone {
			continue
		}
		url := strings.TrimSpace(loc.URL)
		if url == "" {
			url = "/"
		}
		refs = append(refs, fmt.Sprintf("%s location %q", prefix, url))
	}
	return refs
}

type zoneJSON struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Valid   []string `json:"valid"`
	MaxSize int      `json:"max_size"`
}

// marshalZones 生成 cache 段的值：每个缓存区一行，按文件缩进对齐。
func marshalZones(zones []model.CacheZone, indent string) ([]byte, error) {
	inner := indent + "  "
	var b strings.Builder
	b.WriteString("[\n")
	for i, z := range zones {
		valid := z.Valid
		if valid == nil {
			valid = []string{}
		}
		item, err := json.Marshal(zoneJSON{Name: z.Name, Path: z.Path, Valid: valid, MaxSize: z.MaxSize})
		if err != nil {
			return nil, err
		}
		b.WriteString(inner)
		b.Write(item)
		if i < len(zones)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(indent + "]")
	return []byte(b.String()), nil
}

// verifyZones 回读校验：结果必须还能解析，且缓存区与期望一致。
func verifyZones(patched []byte, want []model.CacheZone, path string) error {
	got, err := parseZones(patched, path)
	if err != nil {
		return err
	}
	if len(got) != len(want) {
		return fmt.Errorf("cache section has %d zones, want %d", len(got), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(normalizeZone(got[i]), normalizeZone(want[i])) {
			return fmt.Errorf("cache zone %d does not round-trip", i)
		}
	}
	return nil
}

// normalizeZone 把 nil 与空切片统一，避免 reflect.DeepEqual 误判。
func normalizeZone(z model.CacheZone) model.CacheZone {
	if z.Valid == nil {
		z.Valid = []string{}
	}
	return z
}
