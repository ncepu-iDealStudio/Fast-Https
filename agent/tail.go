package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const eventsFile = "events.jsonl"

// readNewLines 从 offset 读取完整行。文件变短（轮转）时从 0 重新读。
// 返回新偏移和每一行的文本（不含换行）。
func readNewLines(monitorDir string, offset int64) (int64, []string, error) {
	path := filepath.Join(monitorDir, eventsFile)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return offset, nil, nil
		}
		return offset, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return offset, nil, fmt.Errorf("events file is a symlink and will not be followed")
	}
	if info.Size() < offset {
		offset = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return offset, nil, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, nil, err
	}
	r := bufio.NewReader(f)
	var lines []string
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			offset += int64(len(line))
			text := string(line[:len(line)-1])
			if len(text) > 0 && text[len(text)-1] == '\r' {
				text = text[:len(text)-1]
			}
			if text != "" {
				lines = append(lines, text)
			}
		}
		if err == io.EOF {
			return offset, lines, nil
		}
		if err != nil {
			return offset, lines, err
		}
	}
}
