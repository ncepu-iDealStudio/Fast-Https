// Package compress 提供静态响应压缩使用的 gzip 与 brotli 辅助函数。
package compress

import (
	"bytes"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
)

// brotliLevel 是在线压缩的质量级别：体积比 gzip 默认级别更小，
// 又不落到 9-11 级那种只适合离线预压缩的 CPU 成本。
const brotliLevel = 4

// CompressBytes 按内容编码压缩数据，encoding 取 "gzip" 或 "br"。
func CompressBytes(data []byte, encoding string) ([]byte, error) {
	switch encoding {
	case "br":
		return CompressBytesBrotli(data)
	case "gzip":
		return CompressBytesGzip(data)
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}

func CompressBytesGzip(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	_, err := gz.Write(data)
	if err != nil {
		return nil, err
	}

	err = gz.Close()
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// CompressBytesBrotli 用 brotli 压缩数据，对应 Content-Encoding: br。
func CompressBytesBrotli(data []byte) ([]byte, error) {
	var b bytes.Buffer
	w := brotli.NewWriterLevel(&b, brotliLevel)

	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func DecompressBytesGzip(compressed []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}
