package compress

import (
	"bytes"
	"strings"
	"testing"

	"fast-https/modules/compress"
	"github.com/klauspost/compress/gzip"
)

func TestCsGzip(t *testing.T) {
	input := []byte("Hello, World!")

	compressed, err := compress.CompressBytesGzip(input)
	if err != nil {
		t.Fatalf("CompressBytesGzip() error: %v", err)
	}

	// 往返校验（gzip.Reader 保留在测试侧，生产路径无解压需求）
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := make([]byte, len(input))
	if _, err := readFull(r, got); err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("round-trip = %q, want %q", got, input)
	}
}

func readFull(r interface{ Read([]byte) (int, error) }, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if total == len(buf) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// 空输入与高重复输入的压缩边界
func TestCompressBytesGzipEdgeInputs(t *testing.T) {
	if _, err := compress.CompressBytesGzip(nil); err != nil {
		t.Errorf("nil input: %v", err)
	}
	big := []byte(strings.Repeat("a", 1<<20))
	out, err := compress.CompressBytesGzip(big)
	if err != nil {
		t.Errorf("1MB input: %v", err)
	}
	if len(out) > 4096 {
		t.Errorf("1MB of 'a' compressed to %d bytes, want < 4096", len(out))
	}
}
