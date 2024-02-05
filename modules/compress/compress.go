package compress

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/andybalholm/brotli"
)

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

func CompressBytesBr(data []byte) ([]byte, error) {

	var compressed bytes.Buffer

	writer := brotli.NewWriterLevel(&compressed, brotli.BestCompression)

	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return compressed.Bytes(), nil

}

func DecompressBytesBr(compressed []byte) ([]byte, error) {
	var decompressed bytes.Buffer

	reader := brotli.NewReader(bytes.NewReader(compressed))

	_, err := io.Copy(&decompressed, reader)
	if err != nil {
		return nil, err
	}
	return decompressed.Bytes(), nil
}
