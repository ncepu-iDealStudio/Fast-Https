// Package packaging 检查打包进 Linux 包的文件必须使用 LF 行尾。
package packaging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这些文件会被 dpkg/rpm 直接 exec，或由 Linux 工具逐行解析。
// Windows 开发机上 core.autocrlf=true 会把工作区转成 CRLF，
// shebang 变成 "#!/bin/sh\r"，内核于是去找 /bin/sh\r，找不到就报
// ENOENT、退出码 2——表现是装得上、卸载不掉，且报错说的是
// "无法执行 prerm：没有那个文件或目录"（文件明明在）。
func TestPackagingScriptsUseLF(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := []string{
		filepath.Join(root, "build", "packaging", "linux", "*.sh"),
		filepath.Join(root, "build", "packaging", "systemd", "*"),
		filepath.Join(root, "build", "packaging", "repo", "*.sh"),
		filepath.Join(root, "build", "packaging", "repo", "*", "*.sh"),
		filepath.Join(root, "build", "spec"),
		filepath.Join(root, "build", "Dockerfile"),
		filepath.Join(root, "build", "docker", "*"),
	}

	checked := 0
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			checked++
			if strings.Contains(string(data), "\r") {
				t.Errorf("%s 含 CR：维护脚本的 shebang 会变成 \"#!/bin/sh\\r\"，dpkg 执行时"+
					"报 ENOENT；Dockerfile 的续行与 RUN 也会被 \\r 破坏。"+
					"在 .gitattributes 里声明 text eol=lf 后重新检出该文件", path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("没有匹配到任何打包文件，路径可能写错了")
	}
}
