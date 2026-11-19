package acmecmd

import (
	"os"
	"testing"
)

// TestMain 把工作目录切到仓库根：这些测试用 config/… 这样的相对路径读写
// 配置，而包目录现在是 cmd/internal/acmecmd。状态目录同时指到临时目录，
// 避免 acme setup 经过配置事务时在仓库里落 state/。
func TestMain(m *testing.M) {
	if err := os.Chdir("../../.."); err != nil {
		panic(err)
	}
	stateDir, err := os.MkdirTemp("", "fast-https-acmecmd-state-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FASTHTTPS_STATE_DIR", stateDir)
	code := m.Run()
	_ = os.RemoveAll(stateDir)
	os.Exit(code)
}
