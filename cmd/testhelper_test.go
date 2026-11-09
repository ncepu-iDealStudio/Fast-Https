package cmd

import "io"

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
