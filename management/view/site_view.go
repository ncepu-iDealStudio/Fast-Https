package view

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"fast-https/config"
	"fast-https/infra/jsonc"
	"fast-https/management/redact"
	"fast-https/modules/site"
)

// SiteRow 是 site list 的一行。
type SiteRow struct {
	Name       string   `json:"name"`
	File       string   `json:"file"`
	Listen     string   `json:"listen"`
	ServerName string   `json:"server_name"`
	Locations  []string `json:"locations"`
	Error      string   `json:"error,omitempty"`
}

// SiteListData 是 site list 的数据。
type SiteListData struct {
	IncludeDirs []string  `json:"include_dirs"`
	Sites       []SiteRow `json:"sites"`
}

// SiteList 列出 http.include 目录下的站点文件。没有可用目录时返回空列表并给出告警。
func SiteList(mainPath string) (SiteListData, []string, error) {
	dirs, err := config.SiteDirsFromFile(mainPath)
	if err != nil {
		return SiteListData{}, nil, err
	}
	data := SiteListData{IncludeDirs: dirs, Sites: []SiteRow{}}
	if data.IncludeDirs == nil {
		data.IncludeDirs = []string{}
	}
	if len(dirs) == 0 {
		return data, []string{"http.include has no usable directory"}, nil
	}

	entries, err := site.List(dirs)
	if err != nil {
		return data, nil, err
	}
	for _, e := range entries {
		row := SiteRow{Name: e.Name, File: e.File, Listen: e.Listen, ServerName: e.ServerName,
			Locations: e.Types, Error: e.Invalid}
		if row.Locations == nil {
			row.Locations = []string{}
		}
		data.Sites = append(data.Sites, row)
	}
	return data, nil, nil
}

// SiteShowData 是 site show 的数据。Server 已脱敏，注释在 JSON 里不保留。
type SiteShowData struct {
	Name   string         `json:"name"`
	Source string         `json:"source"`
	Merged bool           `json:"merged"`
	Server map[string]any `json:"server"`
}

// SiteShow 返回站点的 server 块：默认是站点文件本身，merged 为真时是合并后的完整 server 块。
// 找不到时返回 ErrNotFound。
func SiteShow(mainPath, name string, merged bool) (SiteShowData, error) {
	name = strings.TrimSpace(name)
	// 名字会拼进文件路径，路径分隔符与 .. 一律拒绝。
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return SiteShowData{}, fmt.Errorf("%w: %q", ErrBadName, name)
	}
	data := SiteShowData{Name: name, Merged: merged}

	if !merged {
		if dirs, err := config.SiteDirsFromFile(mainPath); err == nil {
			if path, ok := site.Find(dirs, name); ok {
				raw, err := os.ReadFile(path)
				if err != nil {
					return data, err
				}
				var obj map[string]any
				if err := jsonc.Unmarshal(raw, &obj); err != nil {
					return data, &InvalidError{Message: fmt.Sprintf("parse %s failed: %v", path, err)}
				}
				data.Source = path
				data.Server = asObject(redact.Value(obj))
				return data, nil
			}
		}
	}

	content, source, err := config.SnapshotServer(mainPath, name)
	if err != nil {
		if errors.Is(err, config.ErrNoServerBlock) {
			return data, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return data, err
	}
	var obj map[string]any
	if err := json.Unmarshal(content, &obj); err != nil {
		return data, err
	}
	data.Source = source
	data.Server = asObject(redact.Value(obj))
	return data, nil
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
