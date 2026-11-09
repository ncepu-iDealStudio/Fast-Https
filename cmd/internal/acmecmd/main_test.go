package acmecmd

import (
	"os"
	"testing"
)

// TestMain 把工作目录切到仓库根：这些测试用 config/… 这样的相对路径读写
// 配置，而包目录现在是 cmd/internal/acmecmd。
func TestMain(m *testing.M) {
	if err := os.Chdir("../../.."); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
