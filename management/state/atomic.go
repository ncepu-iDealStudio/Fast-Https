package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFile 先写同目录的 .tmp，再改名为目标文件。
// 读者只读目标路径：要么是完整的旧内容，要么是完整的新内容，不会看到半个文件。
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = 0o600
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReadFile 只读目标路径，忽略残留的 .tmp。
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteFile(path, data, 0o600)
}

func readJSON(path string, v any) error {
	data, err := ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
