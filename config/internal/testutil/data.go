// Package testutil 提供配置模块测试共用的数据文件定位与解析工具。
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.yaml.in/yaml/v3"
)

// DataDir 返回 config/testdata 目录的绝对路径。
func DataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// LoadYAML 读取 testdata 下的 YAML 文件并解析到 v。
// name 使用斜杠分隔的相对路径，例如 "loader/zip.yaml"。
func LoadYAML(t *testing.T, name string, v any) {
	t.Helper()
	path := filepath.Join(DataDir(), filepath.FromSlash(name))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test data %s: %v", name, err)
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		t.Fatalf("parse test data %s: %v", name, err)
	}
}
