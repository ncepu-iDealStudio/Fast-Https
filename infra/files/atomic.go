// Package files 提供文件写入的基础设施。
//
// 配置文件、证书这类"读者随时可能重新加载"的文件必须原子替换：先写同目录的
// 临时文件再 rename，读者要么看到旧内容，要么看到完整的新内容，不会读到
// 写了一半的字节。rename 在同一个目录内是原子的。
package files

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic 原子写入 path：先写 path+".tmp"，再 rename 到目标。
// 父目录不存在时自动创建；失败会清掉临时文件，不留残骸。
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}
