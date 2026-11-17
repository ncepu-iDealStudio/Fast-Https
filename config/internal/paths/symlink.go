package paths

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FirstSymlinkUnder 返回 root 下（不含 root 自身）第一个符号链接的路径。
// root 本身是链接时先解析，避免把部署方故意链接的站点目录当成违规。
// root 不存在或不可读时返回错误，调用方按"不阻断"处理。
func FirstSymlinkUnder(root string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	var found string
	err = filepath.WalkDir(realRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == realRoot {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
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
