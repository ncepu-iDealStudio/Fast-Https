package cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/andybalholm/brotli"
	"io"
)

func CompressBytes_Gzip(data []byte) ([]byte, error) {
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

func DecompressBytes_Gzip(compressed []byte) ([]byte, error) {
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

func CompressBytes_Br(data []byte) ([]byte, error) {

	var compressed bytes.Buffer

	// 创建一个 Brotli 压缩器，并将其与缓冲区关联
	writer := brotli.NewWriterLevel(&compressed, brotli.BestCompression)

	// 将原始数据写入 Brotli 压缩器
	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}

	// 结束压缩并刷新缓冲区
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return compressed.Bytes(), nil

}

func DecompressBytes_Br(compressed []byte) ([]byte, error) {
	var decompressed bytes.Buffer

	// 创建一个 Brotli 解压缩器，并将其与缓冲区关联
	reader := brotli.NewReader(bytes.NewReader(compressed))

	// 将压缩数据解压缩到缓冲区
	_, err := io.Copy(&decompressed, reader)
	if err != nil {
		return nil, err
	}
	return decompressed.Bytes(), nil
}

func TestCsGzip() {
	// 示例用法
	input := []byte("Hello, World!") // 要压缩的字节数组
	fmt.Println(input)

	compressed, err := CompressBytes_Gzip(input)
	if err != nil {
		fmt.Println("压缩出错:", err)
		return
	}

	fmt.Println("压缩后的数据:", compressed)

	mm, _ := DecompressBytes_Gzip(compressed)
	fmt.Println(mm)
}

func TestCsBr() {
	// 示例用法
	input := []byte("Hello, World!") // 要压缩的字节数组
	fmt.Println(input)

	compressed, err := CompressBytes_Br(input)
	if err != nil {
		fmt.Println("压缩出错:", err)
		return
	}

	fmt.Println("压缩后的数据:", compressed)

	mm, _ := DecompressBytes_Br(compressed)
	fmt.Println(mm)
}
