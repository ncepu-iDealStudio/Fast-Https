// Package cfgdiff 比较两份 JSONC 配置。敏感键只标 changed，不带出新旧值。
package cfgdiff

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"fast-https/infra/jsonc"
	"fast-https/management/redact"
	"fast-https/management/view"
)

const (
	OpAdded    = "added"
	OpRemoved  = "removed"
	OpModified = "modified"
)

// Change 是一条路径级差异。
type Change struct {
	Path    string `json:"path"`
	Op      string `json:"op"`
	Value   any    `json:"value,omitempty"`
	Old     any    `json:"old,omitempty"`
	New     any    `json:"new,omitempty"`
	Changed bool   `json:"changed,omitempty"`
}

// Result 是一次比较的结果。无法解析时返回错误，不返回空差异冒充一致。
type Result struct {
	Identical bool     `json:"identical"`
	Changes   []Change `json:"changes"`
}

// Files 比较两个 JSONC 文件。
func Files(currentPath, candidatePath string) (Result, error) {
	a, err := load(currentPath)
	if err != nil {
		return Result{}, err
	}
	b, err := load(candidatePath)
	if err != nil {
		return Result{}, err
	}
	var changes []Change
	diffValue("", a, b, &changes)
	if changes == nil {
		changes = []Change{}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return Result{Identical: len(changes) == 0, Changes: changes}, nil
}

func load(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := jsonc.Unmarshal(raw, &v); err != nil {
		return nil, &view.InvalidError{Message: fmt.Sprintf("parse %s failed: %v", path, err)}
	}
	return v, nil
}

func diffValue(path string, a, b any, out *[]Change) {
	if equalJSON(a, b) {
		return
	}
	am, aMap := asMap(a)
	bm, bMap := asMap(b)
	if aMap && bMap {
		diffMap(path, am, bm, out)
		return
	}
	as, aArr := asSlice(a)
	bs, bArr := asSlice(b)
	if aArr && bArr {
		diffSlice(path, as, bs, out)
		return
	}
	*out = append(*out, modified(path, a, b))
}

func diffMap(path string, a, b map[string]any, out *[]Change) {
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
	for _, k := range names {
		child := join(path, k)
		av, aok := a[k]
		bv, bok := b[k]
		switch {
		case aok && !bok:
			*out = append(*out, removed(child, k, av))
		case !aok && bok:
			*out = append(*out, added(child, k, bv))
		default:
			if redact.IsSensitiveKey(k) {
				if !equalJSON(av, bv) {
					*out = append(*out, Change{Path: child, Op: OpModified, Changed: true})
				}
				continue
			}
			diffValue(child, av, bv, out)
		}
	}
}

func diffSlice(path string, a, b []any, out *[]Change) {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		child := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case i >= len(a):
			*out = append(*out, Change{Path: child, Op: OpAdded, Value: b[i]})
		case i >= len(b):
			*out = append(*out, Change{Path: child, Op: OpRemoved, Old: a[i]})
		default:
			diffValue(child, a[i], b[i], out)
		}
	}
}

func added(path, key string, v any) Change {
	if redact.IsSensitiveKey(key) {
		return Change{Path: path, Op: OpAdded, Changed: true}
	}
	return Change{Path: path, Op: OpAdded, Value: v}
}

func removed(path, key string, v any) Change {
	if redact.IsSensitiveKey(key) {
		return Change{Path: path, Op: OpRemoved, Changed: true}
	}
	return Change{Path: path, Op: OpRemoved, Old: v}
}

func modified(path string, oldv, newv any) Change {
	leaf := path
	if i := strings.LastIndexAny(path, ".["); i >= 0 {
		leaf = path[i+1:]
		leaf = strings.TrimSuffix(leaf, "]")
	}
	if redact.IsSensitiveKey(leaf) {
		return Change{Path: path, Op: OpModified, Changed: true}
	}
	return Change{Path: path, Op: OpModified, Old: oldv, New: newv}
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func equalJSON(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}
