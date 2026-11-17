package static

import (
	"os"
	"path/filepath"
	"strings"

	"fast-https/config"
)

// symlinkBlocked 判断 reject 策略下该路径是否应按 404 处理。
//
// reject 的语义是"静态根下不允许链接"：从 root 到目标文件的每一段都做
// Lstat，命中符号链接（含 Windows junction）即拒绝。root 自身是链接不算，
// 部署方可以把整个站点目录挂到别处。
//
// 与 check-at-load 的差别只在时机：两者都禁止链接，check-at-load 在加载
// 配置时扫描一次，reject 每次请求都校验，运行期新加的链接也能挡住。
func symlinkBlocked(policy, root, full string) bool {
	if policy != config.SYMLINK_REJECT {
		return false
	}
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, filepath.Clean(full))
	if err != nil || rel == "." {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	current := cleanRoot
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		fi, err := os.Lstat(current)
		if err != nil {
			// 不存在的部分交给打开流程，结果同样是 404。
			return false
		}
		if isLink(current, fi) {
			return true
		}
	}
	return false
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
