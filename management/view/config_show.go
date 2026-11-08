package view

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fast-https/config"
	"fast-https/infra/jsonc"
	"fast-https/management/redact"
)

// ConfigSections 是 config show --section 允许的取值。
var ConfigSections = []string{"http", "acme", "security", "waf", "sites"}

// ConfigShowInput 描述一次 config show。PidFile 只用来给出「改过未重载」的提示。
type ConfigShowInput struct {
	MainPath string
	Section  string
	PidFile  string
}

// ConfigShow 读取磁盘上的主配置与 include 站点，返回脱敏后的数据。
// 注意来源是磁盘：运行中进程实际生效的配置只存在于它的内存里，
// 两者可能不一致，config_dirty_hint 只是按修改时间做的提示。
func ConfigShow(in ConfigShowInput) (map[string]any, []string, error) {
	if in.Section != "" && !isSection(in.Section) {
		return nil, nil, fmt.Errorf("%w %q (want one of %s)", ErrBadSection, in.Section, strings.Join(ConfigSections, ", "))
	}

	main, err := loadObject(in.MainPath)
	if err != nil {
		return nil, nil, err
	}
	absMain, _ := filepath.Abs(in.MainPath)

	var warnings []string
	sites, siteFiles, siteWarnings, err := loadSites(in.MainPath, main)
	if err != nil {
		return nil, nil, err
	}
	warnings = append(warnings, siteWarnings...)

	data := map[string]any{
		"source":      "disk",
		"config_file": absMain,
		"files":       append([]string{absMain}, siteFiles...),
	}

	switch in.Section {
	case "":
		data["config"] = redact.Value(main)
		data["sites"] = sites
	case "sites":
		data["section"] = "sites"
		data["sites"] = sites
	default:
		data["section"] = in.Section
		cfg := map[string]any{}
		if v, ok := main[in.Section]; ok {
			cfg[in.Section] = redact.Value(v)
		} else {
			warnings = append(warnings, fmt.Sprintf("section %q is not present in the config file", in.Section))
		}
		data["config"] = cfg
	}

	if dirty, ok := configDirtyHint(in.PidFile, append([]string{absMain}, siteFiles...)); ok {
		data["config_dirty_hint"] = dirty
	}
	return data, warnings, nil
}

func isSection(name string) bool {
	for _, s := range ConfigSections {
		if s == name {
			return true
		}
	}
	return false
}

// loadObject 读取一个 JSONC 文件并解成对象。读不到的错误保持原样（可用 fs.ErrNotExist 判断），
// 内容有问题的返回 InvalidError。
func loadObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := jsonc.Unmarshal(raw, &obj); err != nil {
		return nil, &InvalidError{Message: fmt.Sprintf("parse %s failed: %v", path, err)}
	}
	if obj == nil {
		return nil, &InvalidError{Message: fmt.Sprintf("%s is not a JSON object", path)}
	}
	return obj, nil
}

// loadSites 读取 http.include 目录下的站点文件（只看目录里直接的 *.json，与 site list 一致）。
// 单个站点读不出来不影响其它站点，记在该站点的 error 字段里。
func loadSites(mainPath string, main map[string]any) ([]map[string]any, []string, []string, error) {
	dirs, err := config.SiteDirsFromFile(mainPath)
	if err != nil {
		return nil, nil, nil, err
	}

	var warnings []string
	if http, ok := main["http"].(map[string]any); ok {
		if includes, ok := http["include"].([]any); ok && len(includes) > len(dirs) {
			warnings = append(warnings, "some http.include entries are not existing directories and were not expanded")
		}
	}

	sites := []map[string]any{}
	var files []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			path := filepath.Join(dir, e.Name())
			files = append(files, path)
			site := map[string]any{
				"name": strings.TrimSuffix(e.Name(), ".json"),
				"file": path,
			}
			if obj, err := loadObject(path); err != nil {
				site["error"] = err.Error()
			} else {
				site["server"] = redact.Value(obj)
			}
			sites = append(sites, site)
		}
	}
	sort.SliceStable(sites, func(i, j int) bool { return sites[i]["name"].(string) < sites[j]["name"].(string) })
	sort.Strings(files)
	return sites, files, warnings, nil
}

// ConfigDrift 报告磁盘上的主配置和站点文件是否比 pid 文件新。
// known 为假表示没有可比较的 pid 文件，不是「没有漂移」。
func ConfigDrift(mainPath, pidFile string) (dirty, known bool, err error) {
	if strings.TrimSpace(pidFile) == "" {
		return false, false, nil
	}
	main, err := loadObject(mainPath)
	if err != nil {
		return false, false, err
	}
	_, files, _, err := loadSites(mainPath, main)
	if err != nil {
		return false, false, err
	}
	absMain, _ := filepath.Abs(mainPath)
	dirty, known = configDirtyHint(pidFile, append([]string{absMain}, files...))
	return dirty, known, nil
}

// configDirtyHint 在 pid 文件存在时，按「配置文件比 pid 文件新」判断是否改过未重载。
// pid 是启动时写的，改配置不会动它。没有 pid 文件时无从判断，ok 为 false。
func configDirtyHint(pidFile string, files []string) (dirty, ok bool) {
	pidInfo, err := os.Stat(pidFile)
	if err != nil {
		return false, false
	}
	for _, f := range files {
		if info, err := os.Stat(f); err == nil && info.ModTime().After(pidInfo.ModTime()) {
			return true, true
		}
	}
	return false, true
}
