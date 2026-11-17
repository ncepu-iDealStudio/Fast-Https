package paths

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// FirstSymlinkUnder 返回 root 下（不含 root 自身）第一个链接的路径。
// root 自身是链接时先解析，避免把部署方故意链接的站点目录当成违规；
// root 不存在或不可读时返回错误，调用方按"不阻断"处理。
func FirstSymlinkUnder(root string) (string, error) {
	start, err := resolveRoot(root)
	if err != nil {
		return "", err
	}
	var found string
	err = filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == start {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if isLink(path, info) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return found, nil
}

// resolveRoot 逐层解开 root 自身可能存在的链接。
// 不用 EvalSymlinks：它不解析 Windows junction。
func resolveRoot(root string) (string, error) {
	current := filepath.Clean(root)
	for i := 0; i < 40; i++ {
		fi, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if !isLink(current, fi) {
			return current, nil
		}
		target, err := os.Readlink(current)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		current = target
	}
	return "", errors.New("too many levels of links")
}

// isLink 识别符号链接与 Windows junction。
// junction（IO_REPARSE_TAG_MOUNT_POINT）在部分 Go/Windows 组合上既不是
// ModeSymlink，也不会被 EvalSymlinks 解析，只能靠 Lstat 的 ModeIrregular
// 加 Readlink 成功来判定。
func isLink(path string, fi os.FileInfo) bool {
	if fi.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if fi.Mode()&os.ModeIrregular == 0 {
		return false
	}
	_, err := os.Readlink(path)
	return err == nil
}
