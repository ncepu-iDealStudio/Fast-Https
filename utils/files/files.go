package files

import (
	"os"
)

// 读取文件内容并返回字节切片
func ReadFile(filename string) ([]byte, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// 将字节切片写入文件
func WriteFile(filename string, data []byte) error {
	err := os.WriteFile(filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// 打开文件并返回文件指针和错误信息
func OpenFile(filename string) (*os.File, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// 关闭文件
func CloseFile(file *os.File) error {
	err := file.Close()
	if err != nil {
		return err
	}
	return nil
}
