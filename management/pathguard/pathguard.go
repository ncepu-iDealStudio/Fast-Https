// Package pathguard 规范化并限制站点 --root 等路径，防止把配置/证书/状态/日志当文档根。
package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPath 表示路径不合法或越出允许范围。
var ErrPath = errors.New("invalid path")

// CheckRoot 把 --root 收成绝对路径：拒绝 ".."、必须是已存在的目录，
// 不得等于或落在 banned 目录（配置、证书、状态、日志）里。符号链接按解析后的真实路径判断。
func CheckRoot(root string, banned []string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("%w: --root is required", ErrPath)
	}
	if containsDotDot(root) {
		return "", fmt.Errorf("%w: path must not contain '..'", ErrPath)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPath, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrPath, err)
		}
		abs = real
		info, err = os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrPath, err)
		}
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrPath, abs)
	}
	for _, b := range banned {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		bAbs, err := filepath.Abs(b)
		if err != nil {
			continue
		}
		if real, err := filepath.EvalSymlinks(bAbs); err == nil {
			bAbs = real
		}
		if sameOrInside(bAbs, abs) {
			return "", fmt.Errorf("%w: document root must not be %s or a directory inside it", ErrPath, bAbs)
		}
	}
	return abs, nil
}

func containsDotDot(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	for _, p := range strings.Split(clean, "/") {
		if p == ".." {
			return true
		}
	}
	return strings.Contains(filepath.ToSlash(path), "/../") || strings.HasPrefix(filepath.ToSlash(path), "../")
}

func sameOrInside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))
}
