package static

import (
	"fast-https/config"
	"path/filepath"
	"strings"
)

// symlinkBlocked 判断在 reject 策略下是否应把该路径按 404 处理。
//
// 判定用 EvalSymlinks 解析后的真实路径与解析后的 root 比对：链接指向
// root 之外就拒绝。root 内部的链接仍然可读——它们和普通文件一样由配置者
// 自己放进站点目录。路径不存在或解析失败时不在这里拦，交给后续打开流程，
// 结果同样是 404。
func symlinkBlocked(policy, root, full string) bool {
	if policy != config.SYMLINK_REJECT {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	realFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, realFull)
	if err != nil {
		return false
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
