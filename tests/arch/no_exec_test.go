package arch_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoExecInProduction 守住「管理命令不执行任意 Shell」。
// cmd 与 management 的生产代码不得引用 os/exec；唯一的例外是
// cmd/daemon_unix.go，它用 exec 拉起本进程的后台子进程，不是执行日志或参数里的命令。
func TestNoExecInProduction(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, dir := range []string{"cmd", "management"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if filepath.Base(path) == "daemon_unix.go" {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Errorf("解析 %s: %v", path, err)
				return nil
			}
			for _, imp := range file.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				if p == "os/exec" {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s 引用了 os/exec；管理与命令代码不能执行任意 Shell", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
