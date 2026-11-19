// Package compress 提供静态响应压缩使用的 gzip 与 brotli 辅助函数。
package compress

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/gzip"
)

// gzipWriterPool 复用 gzip.Writer：klauspost 的 Reset 让同一个 writer
// 可以反复用于不同响应，省掉每请求一次的窗口表分配。
var gzipWriterPool = sync.Pool{
	New: func() interface{} { return gzip.NewWriter(io.Discard) },
}

// brotliLevel 是在线压缩的质量级别：体积比 gzip 默认级别更小，
// 又不落到 9-11 级那种只适合离线预压缩的 CPU 成本。
const brotliLevel = 4

// CompressBytes 按内容编码压缩数据，encoding 取 "gzip" 或 "br"。
func CompressBytes(data []byte, encoding string) ([]byte, error) {
	return CompressReader(bytes.NewReader(data), encoding)
}

// CompressReader 从 r 流式读入并压缩，返回压缩后的完整字节。
// 输入按块读取，不会把原文整体读进内存；输出仍缓冲，调用方需要用它
// 写 Content-Length。大文件因此只多一份压缩体的内存。
func CompressReader(r io.Reader, encoding string) ([]byte, error) {
	var b bytes.Buffer
	switch encoding {
	case "br":
		w := brotli.NewWriterLevel(&b, brotliLevel)
		if _, err := io.Copy(w, r); err != nil {
			_ = w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	case "gzip":
		gz := gzipWriterPool.Get().(*gzip.Writer)
		defer gzipWriterPool.Put(gz)
		gz.Reset(&b)
		if _, err := io.Copy(gz, r); err != nil {
			_ = gz.Close()
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
	return b.Bytes(), nil
}

// CompressBytesGzip 从池里取 writer 压缩数据。
func CompressBytesGzip(data []byte) ([]byte, error) {
	return CompressReader(bytes.NewReader(data), "gzip")
}

// CompressBytesBrotli 用 brotli 压缩数据，对应 Content-Encoding: br。
func CompressBytesBrotli(data []byte) ([]byte, error) {
	return CompressReader(bytes.NewReader(data), "br")
}
