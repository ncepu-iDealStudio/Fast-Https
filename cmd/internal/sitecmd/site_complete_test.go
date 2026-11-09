package sitecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathCompletions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"www", "web", ".cache"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sep := string(os.PathSeparator)

	t.Run("唯一前缀补成目录", func(t *testing.T) {
		matches := pathCompletions(filepath.Join(dir, "ww"))
		if len(matches) != 1 || matches[0] != filepath.Join(dir, "www")+sep {
			t.Fatalf("matches = %v", matches)
		}
	})

	t.Run("多个候选", func(t *testing.T) {
		matches := pathCompletions(filepath.Join(dir, "w"))
		if len(matches) != 2 {
			t.Fatalf("matches = %v", matches)
		}
		if got := commonPrefix(matches); got != filepath.Join(dir, "w") {
			t.Fatalf("commonPrefix = %q", got)
		}
	})

	t.Run("文件不加分隔符", func(t *testing.T) {
		matches := pathCompletions(filepath.Join(dir, "read"))
		if len(matches) != 1 || matches[0] != filepath.Join(dir, "readme.md") {
			t.Fatalf("matches = %v", matches)
		}
	})

	t.Run("默认不列隐藏项", func(t *testing.T) {
		for _, match := range pathCompletions(dir + sep) {
			if filepath.Base(match) == ".cache" {
				t.Fatalf("隐藏目录不该出现在候选里：%v", match)
			}
		}
	})

	t.Run("点开头才列隐藏项", func(t *testing.T) {
		matches := pathCompletions(filepath.Join(dir, ".ca"))
		// filepath.Base 会吃掉结尾分隔符，所以分隔符单独判断。
		if len(matches) != 1 || filepath.Base(matches[0]) != ".cache" || !strings.HasSuffix(matches[0], sep) {
			t.Fatalf("matches = %v", matches)
		}
	})

	t.Run("目录不存在返回空", func(t *testing.T) {
		if matches := pathCompletions(filepath.Join(dir, "nope", "x")); len(matches) != 0 {
			t.Fatalf("matches = %v", matches)
		}
	})
}

func TestCommonPrefix(t *testing.T) {
	sep := string(os.PathSeparator)
	cases := []struct {
		items []string
		want  string
	}{
		{[]string{"/var/www", "/var/web"}, "/var/w"},
		// 分岔点落在根分隔符上；用 sep 拼，Windows 下才是合法路径。
		{[]string{sep + "a", sep + "b"}, sep},
		{[]string{"blog", "blog-api"}, "blog"},
		{[]string{"one"}, "one"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := commonPrefix(c.items); got != c.want {
			t.Fatalf("commonPrefix(%v) = %q, want %q", c.items, got, c.want)
		}
	}
}
