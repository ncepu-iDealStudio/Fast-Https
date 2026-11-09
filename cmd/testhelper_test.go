package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// lineReader 按行返回预置输入，用来驱动向导类的逐行问答。
// acme_test.go 迁到 internal/acmecmd 后，cmd 包内的 site 测试仍需要它。

type lineReader struct {
	lines []string
	next  int
}

func newLineReader(lines ...string) *lineReader {
	readied := make([]string, 0, len(lines))
	for _, line := range lines {
		readied = append(readied, line+"\n")
	}
	return &lineReader{lines: readied}
}

func (r *lineReader) Read(p []byte) (int, error) {
	if r.next >= len(r.lines) {
		return 0, io.EOF
	}
	n := copy(p, r.lines[r.next])
	r.next++
	return n, nil
}

// siteTestEnv 建一个临时的主配置与 conf.d 目录，供 CLI 契约测试使用。
// site 命令组迁到 internal/sitecmd 后，cmd 侧的契约测试仍需要这个夹具。
func siteTestEnv(t *testing.T) (mainPath, includeDir string) {
	t.Helper()
	base := t.TempDir()
	includeDir = filepath.Join(base, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath = filepath.Join(base, "fast-https.json")
	payload := map[string]any{
		"http": map[string]any{
			"include": []any{includeDir},
			"server": []any{
				map[string]any{
					"listen":      80,
					"server_name": "main.example",
					"location": []any{
						map[string]any{"url": "/", "type": "local", "root": "./httpdoc/root"},
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return mainPath, includeDir
}
