package cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

func CompressBytes(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	// 压缩数据
	_, err := gz.Write(data)
	if err != nil {
		return nil, err
	}

	// 关闭gzip压缩器
	err = gz.Close()
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func DecompressBytes(compressed []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// 读取解压缩后的数据
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}

func TestCsGzip() {
	// 示例用法
	input := []byte("Hello, World!") // 要压缩的字节数组
	fmt.Println(input)

	compressed, err := CompressBytes(input)
	if err != nil {
		fmt.Println("压缩出错:", err)
		return
	}

	fmt.Println("压缩后的数据:", compressed)

	mm, _ := DecompressBytes(compressed)
	fmt.Println(mm)
}
