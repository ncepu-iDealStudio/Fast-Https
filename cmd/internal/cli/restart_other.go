//go:build !windows

package cli

import "errors"

// restartService 只在 Windows 的 reload 回退路径上被调用：其它平台
// SignalReload 可用，不会走到这里。保留同签名是为了让 txn.go 的平台
// 分支无需额外构建标签。
func restartService(string) error {
	return errors.New("restart fallback is only available on Windows")
}
