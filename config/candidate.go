package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrCandidatePath 表示候选路径含 ".." 或解析后（含符号链接）逃出候选树。
// CLI 把它映射成 USAGE_ERROR（退出码 2）。
var ErrCandidatePath = errors.New("invalid candidate path")

// ResolveCandidateRoot 规范化候选路径：拒绝 ".." 逃逸，目录或主配置文件都落到树根。
// 候选根自身若是符号链接，按解析后的真实目录作为树根。
func ResolveCandidateRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: candidate path is empty", ErrCandidatePath)
	}
	if containsDotDot(path) {
		return "", fmt.Errorf("%w: candidate path must not contain '..'", ErrCandidatePath)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCandidatePath, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("%w: resolve candidate symlink: %v", ErrCandidatePath, err)
		}
		abs = resolved
		info, err = os.Stat(abs)
		if err != nil {
			return "", err
		}
	}
	if info.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}

func containsDotDot(path string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	for _, part := range strings.Split(cleaned, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// withinRoot 要求 target 解析后仍落在 root 内。符号链接指向目录外时失败。
func withinRoot(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCandidatePath, err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCandidatePath, err)
	}
	rootEval := evalExisting(rootAbs)
	targetEval := evalExisting(targetAbs)
	rel, err := filepath.Rel(rootEval, targetEval)
	if err != nil {
		return fmt.Errorf("%w: %s is outside candidate tree", ErrCandidatePath, target)
	}
	relSlash := filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(relSlash, "../") {
		return fmt.Errorf("%w: %s is outside candidate tree", ErrCandidatePath, target)
	}
	return nil
}

func evalExisting(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	dir := filepath.Dir(p)
	if dir == p {
		return filepath.Clean(p)
	}
	return filepath.Join(evalExisting(dir), filepath.Base(p))
}
