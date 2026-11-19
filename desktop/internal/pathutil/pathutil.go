// Package pathutil 在「数据目录」基准下展开与收回配置里的相对路径。
//
// 配置为了可搬迁保留相对写法，界面需要绝对路径才能消除歧义：
// 展示前用 Resolve 展开，保存时用 Compact 收回数据目录内的路径。
package pathutil

import (
	"path/filepath"
	"strings"
)

// Resolve 把相对路径按数据目录展开成绝对路径；绝对路径只做 Clean。
func Resolve(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

// Compact 把数据目录内的绝对路径收回成 ./相对路径，保持配置可搬迁；
// 目录外的绝对路径、本就是相对写法的路径原样返回。
func Compact(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(base, filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Clean(path)
	}
	return "./" + filepath.ToSlash(rel)
}
