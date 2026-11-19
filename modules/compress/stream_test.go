package compress

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/andybalholm/brotli"
)

// TestCompressReaderStreamsInput 用一个每次只吐 1 字节的 reader，
// 证明压缩是从输入流按块读，而不是要求调用方先 ReadAll。
func TestCompressReaderStreamsInput(t *testing.T) {
	raw := []byte(strings.Repeat("stream-me ", 500))
	for _, enc := range []string{"gzip", "br"} {
		t.Run(enc, func(t *testing.T) {
			compressed, err := CompressReader(iotest.OneByteReader(bytes.NewReader(raw)), enc)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decode(t, compressed, enc), raw) {
				t.Fatal("round-trip mismatch")
			}
		})
	}
}

// TestCompressReaderEmptyInput 空输入也要产出合法的压缩流。
func TestCompressReaderEmptyInput(t *testing.T) {
	for _, enc := range []string{"gzip", "br"} {
		compressed, err := CompressReader(bytes.NewReader(nil), enc)
		if err != nil {
			t.Fatal(err)
		}
		if got := decode(t, compressed, enc); len(got) != 0 {
			t.Fatalf("%s: got %q, want empty", enc, got)
		}
	}
}

// TestCompressReaderPropagatesReadError 读输入失败要返回错误，不能写出半截流当成功。
func TestCompressReaderPropagatesReadError(t *testing.T) {
	for _, enc := range []string{"gzip", "br"} {
		src := io.MultiReader(strings.NewReader("partial"), errReader{})
		if _, err := CompressReader(src, enc); err == nil {
			t.Fatalf("%s: read error should be returned", enc)
		}
	}
}

func decode(t *testing.T, data []byte, enc string) []byte {
	t.Helper()
	switch enc {
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer zr.Close()
		out, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		return out
	case "br":
		out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(data)))
		if err != nil {
			t.Fatal(err)
		}
		return out
	default:
		t.Fatalf("unknown encoding %q", enc)
		return nil
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }
