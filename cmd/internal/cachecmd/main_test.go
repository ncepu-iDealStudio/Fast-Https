package cachecmd

import (
	"os"
	"testing"
)

// TestMain 把状态目录指到临时目录：缓存区写操作会登记 operation 记录，
// 不隔离的话测试会在包目录下生成 state/。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fast-https-cachecmd-state-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FASTHTTPS_STATE_DIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
