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
// cmd 与 management 的生产代码不得引用 os/exec。允许的例外都只重执行
// 自身进程、参数写死、不经过 Shell，见 allowedSelfExec。
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
			if _, ok := allowedSelfExec[filepath.Base(path)]; ok {
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
					t.Errorf("%s 引用了 os/exec；管理与命令代码不能执行任意 Shell"+
						"（若确实只重执行自身，请加入 allowedSelfExec 并写明理由）", rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// allowedSelfExec 是 os/exec 的白名单：文件只重新执行自身进程，
// 参数是写死的子命令，不读取日志/配置/用户输入，不经过 Shell。
var allowedSelfExec = map[string]string{
	"daemon_unix.go":     "Unix 后台化：exec 自身并移交监听套接字",
	"daemon_windows.go":  "Windows 后台化：DETACHED_PROCESS 拉起自身",
	"restart_windows.go": "Windows 热重载回退：顺序执行自身的 stop / start",
}
