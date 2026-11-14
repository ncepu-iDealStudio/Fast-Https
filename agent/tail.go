package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const eventsFile = "events.jsonl"

// readNewLines 从 offset 读取完整行。
// prefix 是上次看到的文件开头。文件被换掉时，即使新文件更长，也从 0 读。
// 文件变短时同样从 0 读。返回新偏移、新的开头标记和每一行文本。
func readNewLines(monitorDir string, offset int64, prefix string) (int64, string, []string, error) {
	path := filepath.Join(monitorDir, eventsFile)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return offset, prefix, nil, nil
		}
		return offset, prefix, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return offset, prefix, nil, fmt.Errorf("events file is a symlink and will not be followed")
	}
	f, err := os.Open(path)
	if err != nil {
		return offset, prefix, nil, err
	}
	defer f.Close()
	if replaced, id, err := fileReplaced(f, prefix); err != nil {
		return offset, prefix, nil, err
	} else if replaced || info.Size() < offset {
		offset = 0
		prefix = id
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, prefix, nil, err
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
			if next, err := prefixOf(f); err == nil && next != "" {
				prefix = next
			}
			return offset, prefix, lines, nil
		}
		if err != nil {
			return offset, prefix, lines, err
		}
	}
}

func fileReplaced(f *os.File, prefix string) (bool, string, error) {
	if prefix == "" {
		return false, "", nil
	}
	n, sum, err := splitPrefix(prefix)
	if err != nil {
		return true, "", nil
	}
	got, err := hashFirst(f, n)
	if err != nil {
		return true, "", nil
	}
	return got != sum, got, nil
}

func prefixOf(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	n := int(info.Size())
	if n > 64 {
		n = 64
	}
	if n == 0 {
		return "", nil
	}
	sum, err := hashFirst(f, n)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(n) + ":" + sum, nil
}

func splitPrefix(prefix string) (int, string, error) {
	nText, sum, ok := strings.Cut(prefix, ":")
	if !ok {
		return 0, "", fmt.Errorf("bad prefix")
	}
	n, err := strconv.Atoi(nText)
	if err != nil || n < 1 {
		return 0, "", fmt.Errorf("bad prefix")
	}
	return n, sum, nil
}

func hashFirst(f *os.File, n int) (string, error) {
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:8]), nil
}
