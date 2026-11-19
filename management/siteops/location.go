package siteops

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"fast-https/config"
	"fast-https/infra/jsonc"
	"fast-https/management/pathguard"
	"fast-https/modules/site"
)

// LocationInput 是 site location add 的输入。
// 额外 location 只支持基础字段：路径、类型、静态根或上游地址。
type LocationInput struct {
	Site      string
	URL       string
	Type      string // local | proxy
	Root      string
	ProxyPass string
	Banned    []string
}

// LocationUpdateInput 只改显式给出的字段。
type LocationUpdateInput struct {
	Site      string
	URL       string
	Type      *string
	Root      *string
	ProxyPass *string
	Banned    []string
}

// normalizeLocationURL 校验路径写法：必须以 / 开头，不能用空白。
// 主 location "/" 由 site update 管理，这里一律拒绝，避免出现两个主 location。
func normalizeLocationURL(raw string) (string, error) {
	url := strings.TrimSpace(raw)
	if url == "" {
		return "", fmt.Errorf("%w: --url is required", ErrUsage)
	}
	if !strings.HasPrefix(url, "/") || strings.ContainsAny(url, " \t\r\n") {
		return "", fmt.Errorf("%w: location url must start with / and contain no spaces: %q", ErrUsage, raw)
	}
	if url == "/" {
		return "", fmt.Errorf("%w: the main location / is managed by site update; location add/delete only handles extra paths", ErrUsage)
	}
	return url, nil
}

// validateLocationPattern 确认路径能编译成正则：运行期用 MustCompile 建 PathRe，
// 非法模式（例如 "/api("）会让 reload 直接 panic，必须在写盘前拦住。
func validateLocationPattern(url string) error {
	if _, err := regexp.Compile(url); err != nil {
		return fmt.Errorf("%w: location url is not a valid pattern: %v", ErrUsage, err)
	}
	return nil
}

// buildLocation 校验类型与目标，返回可写入的 location。
func buildLocation(in LocationInput) (config.SiteLocation, error) {
	url, err := normalizeLocationURL(in.URL)
	if err != nil {
		return config.SiteLocation{}, err
	}
	if err := validateLocationPattern(url); err != nil {
		return config.SiteLocation{}, err
	}
	switch strings.TrimSpace(in.Type) {
	case "proxy":
		pass := strings.TrimSpace(in.ProxyPass)
		if pass == "" {
			return config.SiteLocation{}, fmt.Errorf("%w: --proxy-pass is required for proxy locations", ErrUsage)
		}
		if !strings.HasPrefix(pass, "http://") && !strings.HasPrefix(pass, "https://") {
			return config.SiteLocation{}, fmt.Errorf("%w: --proxy-pass must start with http:// or https://", ErrUsage)
		}
		return config.SiteLocation{URL: url, Type: "proxy", ProxyPass: pass}, nil
	case "local", "":
		root := strings.TrimSpace(in.Root)
		if root == "" {
			return config.SiteLocation{}, fmt.Errorf("%w: --root is required for local locations", ErrUsage)
		}
		abs, err := pathguard.CheckRoot(root, in.Banned)
		if err != nil {
			return config.SiteLocation{}, err
		}
		return config.SiteLocation{
			URL: url, Type: "local", Root: abs,
			Index: []string{"index.html", "index.htm"},
		}, nil
	default:
		return config.SiteLocation{}, fmt.Errorf("%w: --type accepts local or proxy, got %q", ErrUsage, in.Type)
	}
}

// AddLocation 追加一个额外 location（/api 代理由此配置）。
func AddLocation(dirs []string, liveMain string, in LocationInput) (string, error) {
	if err := checkEditableSite(liveMain, in.Site); err != nil {
		return "", err
	}
	loc, err := buildLocation(in)
	if err != nil {
		return "", err
	}
	return site.Update(dirs, in.Site, func(cur *config.SiteInput) error {
		if findLocationIndex(cur.Locations, loc.URL) >= 0 {
			return fmt.Errorf("%w: location %s already exists", ErrConflict, loc.URL)
		}
		cur.Locations = append(cur.Locations, loc)
		return nil
	})
}

// UpdateLocation 按 url 定位并只改显式给出的字段。
func UpdateLocation(dirs []string, liveMain string, in LocationUpdateInput) (string, error) {
	if err := checkEditableSite(liveMain, in.Site); err != nil {
		return "", err
	}
	url, err := normalizeLocationURL(in.URL)
	if err != nil {
		return "", err
	}
	if err := validateLocationPattern(url); err != nil {
		return "", err
	}
	if in.Type == nil && in.Root == nil && in.ProxyPass == nil {
		return "", fmt.Errorf("%w: location update requires at least one of --type, --root, --proxy-pass", ErrUsage)
	}
	return site.Update(dirs, in.Site, func(cur *config.SiteInput) error {
		idx := findLocationIndex(cur.Locations, url)
		if idx < 0 {
			return fmt.Errorf("%w: location %s", ErrNotFound, url)
		}
		loc := &cur.Locations[idx]
		if in.Type != nil {
			loc.Type = strings.TrimSpace(*in.Type)
		}
		switch loc.Type {
		case "proxy":
			if in.ProxyPass != nil {
				pass := strings.TrimSpace(*in.ProxyPass)
				if pass == "" {
					return fmt.Errorf("%w: --proxy-pass must not be empty", ErrUsage)
				}
				if !strings.HasPrefix(pass, "http://") && !strings.HasPrefix(pass, "https://") {
					return fmt.Errorf("%w: --proxy-pass must start with http:// or https://", ErrUsage)
				}
				loc.ProxyPass = pass
			}
			loc.Root = ""
			loc.Index = nil
			loc.SymlinkPolicy = ""
			if strings.TrimSpace(loc.ProxyPass) == "" {
				return fmt.Errorf("%w: --proxy-pass is required for proxy locations", ErrUsage)
			}
		case "local", "":
			if in.Root != nil {
				abs, err := pathguard.CheckRoot(*in.Root, in.Banned)
				if err != nil {
					return err
				}
				loc.Root = abs
			}
			loc.ProxyPass = ""
			if len(loc.Index) == 0 {
				loc.Index = []string{"index.html", "index.htm"}
			}
			if strings.TrimSpace(loc.Root) == "" {
				return fmt.Errorf("%w: --root is required for local locations", ErrUsage)
			}
		default:
			return fmt.Errorf("%w: --type accepts local or proxy, got %q", ErrUsage, loc.Type)
		}
		return nil
	})
}

// DeleteLocation 按 url 删除一个额外 location；主 location "/" 不允许删除。
func DeleteLocation(dirs []string, liveMain, name, rawURL string) (string, error) {
	if err := checkEditableSite(liveMain, name); err != nil {
		return "", err
	}
	url, err := normalizeLocationURL(rawURL)
	if err != nil {
		return "", err
	}
	path, ok := site.Find(dirs, name)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	cur, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	idx := findLocationIndex(cur.Locations, url)
	if idx < 0 {
		return "", fmt.Errorf("%w: location %s", ErrNotFound, url)
	}
	data, err := jsonc.PatchPath(raw, []jsonc.Edit{{
		Path: []jsonc.Step{jsonc.Key("location"), jsonc.At(idx)},
	}})
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
		return "", fmt.Errorf("rename %s: %w", path, err)
	}
	return path, nil
}

// SetExtraLocations 用给定列表整体替换非主 location（桌面端一次保存）。
// 主 location 的原文一字不动；已存在的额外路径只就地补基础字段，注释与
// try、error_page、proxy_set_header 等未建模字段都保持原样；只有从列表里
// 消失的 URL 才删除，新 URL 才追加。
func SetExtraLocations(dirs []string, liveMain, name string, extras []config.SiteLocation, banned []string) (string, error) {
	if err := checkEditableSite(liveMain, name); err != nil {
		return "", err
	}
	clean := make([]config.SiteLocation, 0, len(extras))
	wanted := make(map[string]bool, len(extras))
	for _, raw := range extras {
		loc, err := buildLocation(LocationInput{
			Site: name, URL: raw.URL, Type: raw.Type, Root: raw.Root, ProxyPass: raw.ProxyPass,
			Banned: banned,
		})
		if err != nil {
			return "", err
		}
		if loc.URL == "/" {
			return "", fmt.Errorf("%w: the main location / is managed by site update", ErrUsage)
		}
		if wanted[loc.URL] {
			return "", fmt.Errorf("%w: location %s appears twice", ErrConflict, loc.URL)
		}
		wanted[loc.URL] = true
		clean = append(clean, loc)
	}

	path, ok := site.Find(dirs, name)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	cur, err := config.ParseSiteInput(raw)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败：%w", path, err)
	}

	var edits []jsonc.Edit
	if len(cur.Locations) == 0 {
		edits = append(edits, jsonc.Edit{Path: []jsonc.Step{jsonc.Key("location")}, Value: []byte("[]")})
	}
	existing := make(map[string]int, len(cur.Locations))
	for i, loc := range cur.Locations {
		u := strings.TrimSpace(loc.URL)
		if u == "" || u == "/" {
			continue
		}
		existing[u] = i
	}

	// 已存在的 URL 原地改基础字段：这一步不改变元素个数，可以放心用原始下标。
	for _, loc := range clean {
		idx, ok := existing[loc.URL]
		if !ok {
			continue
		}
		edits = append(edits, locationBasicEdits(idx, cur.Locations[idx], loc)...)
	}
	// 消失的 URL 倒序删除，避免下标漂移。
	for i := len(cur.Locations) - 1; i >= 0; i-- {
		u := strings.TrimSpace(cur.Locations[i].URL)
		if u == "" || u == "/" || wanted[u] {
			continue
		}
		edits = append(edits, jsonc.Edit{
			Path: []jsonc.Step{jsonc.Key("location"), jsonc.At(i)},
		})
	}
	// 新 URL 追加到数组末尾。
	for _, loc := range clean {
		if _, ok := existing[loc.URL]; ok {
			continue
		}
		value, err := json.Marshal(loc)
		if err != nil {
			return "", err
		}
		edits = append(edits, jsonc.Edit{
			Path:  []jsonc.Step{jsonc.Key("location"), jsonc.Append()},
			Value: value,
		})
	}
	if len(edits) == 0 {
		return path, nil
	}
	data, err := jsonc.PatchPath(raw, edits)
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
		return "", fmt.Errorf("rename %s: %w", path, err)
	}
	return path, nil
}

// locationBasicEdits 只改额外 location 的基础字段，其它键（包括 try、
// error_page、proxy_set_header 等未建模字段）原样保留。
func locationBasicEdits(idx int, old, next config.SiteLocation) []jsonc.Edit {
	base := []jsonc.Step{jsonc.Key("location"), jsonc.At(idx)}
	oldType := strings.TrimSpace(old.Type)
	if oldType == "" {
		oldType = "local"
	}
	var edits []jsonc.Edit
	if oldType != next.Type {
		edits = append(edits, locationMemberEdit(base, "type", next.Type))
	}
	if next.Type == "proxy" {
		if oldType != "proxy" || strings.TrimSpace(old.ProxyPass) != next.ProxyPass {
			edits = append(edits, locationMemberEdit(base, "proxy_pass", next.ProxyPass))
		}
		if strings.TrimSpace(old.Root) != "" {
			edits = append(edits, locationMemberDelete(base, "root"))
		}
		if len(old.Index) > 0 {
			edits = append(edits, locationMemberDelete(base, "index"))
		}
		if strings.TrimSpace(old.SymlinkPolicy) != "" {
			edits = append(edits, locationMemberDelete(base, "symlink_policy"))
		}
		return edits
	}
	if strings.TrimSpace(old.Root) != next.Root {
		edits = append(edits, locationMemberEdit(base, "root", next.Root))
	}
	if strings.TrimSpace(old.ProxyPass) != "" {
		edits = append(edits, locationMemberDelete(base, "proxy_pass"))
	}
	// 代理专用字段在改成静态后不再生效，顺手清掉。
	if strings.TrimSpace(old.ProxyCache) != "" {
		edits = append(edits, locationMemberDelete(base, "proxy_cache"))
	}
	if oldType == "proxy" {
		edits = append(edits, locationMemberDelete(base, "proxy_set_header"))
	}
	if len(old.Index) == 0 {
		edits = append(edits, locationMemberEdit(base, "index", next.Index))
	}
	return edits
}

func locationMemberEdit(base []jsonc.Step, key string, value any) jsonc.Edit {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte("null")
	}
	return jsonc.Edit{Path: locationMemberPath(base, key), Value: data}
}

func locationMemberDelete(base []jsonc.Step, key string) jsonc.Edit {
	return jsonc.Edit{Path: locationMemberPath(base, key)}
}

func locationMemberPath(base []jsonc.Step, key string) []jsonc.Step {
	out := make([]jsonc.Step, 0, len(base)+1)
	out = append(out, base...)
	return append(out, jsonc.Key(key))
}

// ListLocations 返回站点文件里的全部 location。
func ListLocations(dirs []string, name string) ([]config.SiteLocation, error) {
	path, ok := site.Find(dirs, name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	in, err := config.ParseSiteInput(raw)
	if err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	return in.Locations, nil
}

// findLocationIndex 按 url 精确匹配，空 url 视为主 location "/"。
func findLocationIndex(locs []config.SiteLocation, url string) int {
	for i := range locs {
		cur := strings.TrimSpace(locs[i].URL)
		if cur == "" {
			cur = "/"
		}
		if cur == url {
			return i
		}
	}
	return -1
}

// checkEditableSite 拒绝主配置里的内联站点与不存在的站点。
func checkEditableSite(liveMain, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: site name is required", ErrUsage)
	}
	if inlineSite(liveMain, name) {
		return fmt.Errorf("%w: site %s is inline in the main config (http.server); move it to conf.d first", ErrNotSupported, name)
	}
	return nil
}
