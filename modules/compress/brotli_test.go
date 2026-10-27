package compress

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

// brotli 压缩结果必须能解回原文，且对重复文本明显小于原文
func TestCompressBytesBrotliRoundTrip(t *testing.T) {
	raw := []byte(strings.Repeat("hello fast-https brotli ", 100))
	compressed, err := CompressBytesBrotli(raw)
	if err != nil {
		t.Fatalf("CompressBytesBrotli() error: %v", err)
	}
	if len(compressed) >= len(raw) {
		t.Fatalf("compressed %d bytes, want smaller than raw %d", len(compressed), len(raw))
	}

	reader := brotli.NewReader(bytes.NewReader(compressed))
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompress error: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("round-trip mismatch")
	}
}

// CompressBytes 分发器：br/gzip 之外必须报错而不是静默返回
func TestCompressBytesRejectsUnknownEncoding(t *testing.T) {
	if _, err := CompressBytes([]byte("x"), "deflate"); err == nil {
		t.Error("unknown encoding should return an error")
	}
	if _, err := CompressBytes([]byte("x"), "gzip"); err != nil {
		t.Errorf("gzip dispatch error: %v", err)
	}
	if _, err := CompressBytes([]byte("x"), "br"); err != nil {
		t.Errorf("br dispatch error: %v", err)
	}
}
