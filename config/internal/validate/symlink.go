package validate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
)

// 静态根符号链接策略。取值常量与归一化在 model 包，这里只做校验。
//
//   - ""/"check-at-load"（默认）：加载配置时扫描 root，发现符号链接就拒绝加载；
//     请求期零开销。
//   - "allow"：保持旧行为，链接照常解析（含指向 root 之外的链接）。
//   - "reject"：请求路径经 EvalSymlinks 解析后若落在 root 之外则 404，
//     root 内部的链接仍可读。

// validateSymlinkPolicy 校验策略取值；check-at-load 时扫描 root 下的符号链接。
// root 不存在或不可读时跳过扫描：配置校验不因此失败，留给运行期报错。
func validateSymlinkPolicy(policy, root, where string) error {
	policy = model.NormalizeSymlinkPolicy(policy)
	switch policy {
	case model.SYMLINK_ALLOW, model.SYMLINK_REJECT, model.SYMLINK_CHECK_AT_LOAD:
	default:
		return fmt.Errorf("%s: symlink_policy is invalid: %s (want allow, reject or check-at-load)", where, policy)
	}
	if policy != model.SYMLINK_CHECK_AT_LOAD || strings.TrimSpace(root) == "" {
		return nil
	}
	link, err := firstSymlinkUnder(paths.DataPath(root))
	if err != nil || link == "" {
		return nil
	}
	return fmt.Errorf("%s: symlink found under root: %s (symlink_policy=check-at-load; remove the link or set symlink_policy to allow/reject)", where, link)
}

// firstSymlinkUnder 返回 root 下（不含 root 自身）第一个符号链接的路径。
// root 本身是链接时先解析，避免把部署方故意链接的站点目录当成违规。
func firstSymlinkUnder(root string) (string, error) {
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
