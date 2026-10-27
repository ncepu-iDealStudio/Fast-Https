package request

import (
	"bytes"
	"errors"
	"testing"
)

func TestReadChunkedSingleRead(t *testing.T) {
	body, err := ReadChunked([]byte("5\r\nhello\r\n0\r\n\r\n"), func() ([]byte, error) {
		t.Fatal("body was already complete")
		return nil, nil
	}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body %q", body)
	}
}

func TestReadChunkedAcrossReads(t *testing.T) {
	parts := [][]byte{
		[]byte("hello\r\n"),
		[]byte("0\r\n\r\n"),
	}
	body, err := ReadChunked([]byte("5\r\n"), func() ([]byte, error) {
		if len(parts) == 0 {
			return nil, errors.New("eof")
		}
		next := parts[0]
		parts = parts[1:]
		return next, nil
	}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("body %q", body)
	}
}

func TestReadChunkedRejectsBadSizeAndOverflow(t *testing.T) {
	_, err := ReadChunked([]byte("zz\r\n0\r\n\r\n"), func() ([]byte, error) {
		return nil, errors.New("eof")
	}, 1024)
	if !errors.Is(err, ErrChunkedBody) {
		t.Fatalf("bad size err %v", err)
	}
	_, err = ReadChunked([]byte("5\r\nhello\r\n0\r\n\r\n"), func() ([]byte, error) {
		return nil, errors.New("eof")
	}, 4)
	if !errors.Is(err, ErrChunkedBody) {
		t.Fatalf("overflow err %v", err)
	}
}

func TestHasTokenIgnoresZeroQuality(t *testing.T) {
	if !HasToken("deflate, gzip", "gzip") {
		t.Fatal("gzip should be accepted")
	}
	if HasToken("gzip;q=0", "gzip") {
		t.Fatal("gzip q=0 should be rejected")
	}
	if !HasToken("chunked", "chunked") {
		t.Fatal("chunked should match")
	}
}

func TestHeaderFold(t *testing.T) {
	req := RequestInit(false)
	req.Headers["accept-encoding"] = "gzip"
	if req.HeaderFold("Accept-Encoding") != "gzip" {
		t.Fatalf("header %q", req.HeaderFold("Accept-Encoding"))
	}
	if !bytes.Equal([]byte("gzip"), []byte(req.HeaderFold("Accept-Encoding"))) {
		t.Fatal("fold mismatch")
	}
}
