package view

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSiteWorkspace 建一个最小可用的运行期目录：config/fast-https.json
// 加一个空 include 目录。各包的测试互不可见，所以这里自带一份。
func writeSiteWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	etc := filepath.Join(dir, "config")
	confd := filepath.Join(etc, "conf.d")
	if err := os.MkdirAll(confd, 0o755); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(etc, "fast-https.json")
	body := "{\n  \"http\": {\n    \"include\": [\"./conf.d\"],\n    \"server\": []\n  }\n}\n"
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
