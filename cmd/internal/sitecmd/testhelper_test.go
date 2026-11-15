package sitecmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"text/template"
)

// renderTestdata 渲染 testdata 下的模板。固定输入放 testdata（编译器不会
// 把该目录当成包内容），需要按测试临时目录改路径的用 .tmpl 模板。
func renderTestdata(t *testing.T, name string, data any) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	tpl, err := template.New(name).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse testdata/%s: %v", name, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("render testdata/%s: %v", name, err)
	}
	return buf.String()
}

// lineReader 按行返回预置输入，用来驱动向导类的逐行问答。
// 原来在 cmd 包的测试里，site 测试随源码迁出后需要自己的一份。

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
