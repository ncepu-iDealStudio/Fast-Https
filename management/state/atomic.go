package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteFile 先写同目录的唯一临时文件，再改名为目标文件。
// 读者只读目标路径：要么是完整的旧内容，要么是完整的新内容，不会看到半个文件。
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = 0o600
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(tmp, perm)
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
