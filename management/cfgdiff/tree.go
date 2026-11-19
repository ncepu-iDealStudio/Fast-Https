package cfgdiff

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"fast-https/config"
)

// Trees 比较两棵配置目录树里的 JSON 文件（主配置 + conf.d）。
// 跳过 manifest.json 与非 json。无法解析时返回错误，不返回空差异。
func Trees(currentRoot, candidateRoot string) (Result, error) {
	a, err := listJSON(currentRoot)
	if err != nil {
		return Result{}, err
	}
	b, err := listJSON(candidateRoot)
	if err != nil {
		return Result{}, err
	}
	keys := map[string]struct{}{}
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)

	var changes []Change
	for _, rel := range names {
		ap, aok := a[rel]
		bp, bok := b[rel]
		switch {
		case aok && !bok:
			changes = append(changes, Change{Path: rel, Op: OpRemoved})
		case !aok && bok:
			changes = append(changes, Change{Path: rel, Op: OpAdded})
		default:
			sub, err := Files(ap, bp)
			if err != nil {
				return Result{}, err
			}
			for _, c := range sub.Changes {
				c.Path = fileChangePath(rel, c.Path)
				changes = append(changes, c)
			}
		}
	}
	if changes == nil {
		changes = []Change{}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return Result{Identical: len(changes) == 0, Changes: changes}, nil
}

func fileChangePath(rel, jsonPath string) string {
	if jsonPath == "" {
		return rel
	}
	return rel + ":" + jsonPath
}

// listJSON 只收集事务真正管理的配置文件：根目录的 fast-https.json 与
// conf.d 下的站点文件。mime.json、monitor.json 等运行期附属文件既不在
// 候选树里、也不在版本快照里，不能因为缺席就被当成「删除」。
func listJSON(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !config.IsTreeConfigFile(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isManagedConfigPath(rel) {
			return nil
		}
		out[rel] = path
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isManagedConfigPath 判断相对路径是否属于配置事务管理的范围。
func isManagedConfigPath(rel string) bool {
	return rel == "fast-https.json" || strings.HasPrefix(rel, "conf.d/")
}
