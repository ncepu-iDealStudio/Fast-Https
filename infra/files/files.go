// Package files 提供配置加载与响应缓存使用的文件辅助函数。
package files

import (
	"os"
)

// ReadFile 读取文件内容并返回字节切片。
func ReadFile(filename string) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return content, nil
}
