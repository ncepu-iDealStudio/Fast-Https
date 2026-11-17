// Package source 负责把配置树展开成具体的文件：http.include 展开与 MIME 表读取。
// 只读文件，不做校验，也不替换运行期快照。
package source

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fast-https/config/internal/paths"
)

// IncludeFiles 把 http.include 展开成具体的 .json 文件列表：目录递归收集，
// 单个文件必须带 .json 后缀。baseDir 是解析相对路径的基准；treeRoot 非空时
// 拒绝逃出该根（含符号链接）。fallbacks 传给 paths.PathIn，线上入口传 "."。
func IncludeFiles(includes []string, baseDir, treeRoot string, fallbacks ...string) ([]string, error) {
	var files []string
	for _, includePath := range includes {
		if strings.TrimSpace(includePath) == "" {
			return nil, errors.New("http.include must contain non-empty paths")
		}

		fullPath := paths.PathIn(baseDir, includePath, fallbacks...)
		if treeRoot != "" {
			if err := paths.WithinRoot(treeRoot, fullPath); err != nil {
				return nil, err
			}
		}
		info, err := os.Stat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("include path not found: %s", fullPath)
		}

		if !info.IsDir() {
			if filepath.Ext(fullPath) != ".json" {
				return nil, fmt.Errorf("include file must be .json: %s", fullPath)
			}
			files = append(files, fullPath)
			continue
		}

		err = filepath.Walk(fullPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".json" {
				return nil
			}
			if treeRoot != "" {
				if err := paths.WithinRoot(treeRoot, path); err != nil {
					return err
				}
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
