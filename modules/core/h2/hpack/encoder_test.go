package hpack

import (
	assert "github.com/Jxck/assertion"
	"testing"
)

func TestStringLiteralEncode(t *testing.T) {
	// D.2.1.  Literal Header Field with Indexing
	var indexing Indexing = WITH
	var name, value string = "custom-key", "custom-header"
	buf := []byte{
		0x40, 0x0a,
		0x63, 0x75,
		0x73, 0x74,
		0x6f, 0x6d,
		0x2d, 0x6b,
		0x65, 0x79,
		0x0d, 0x63,
		0x75, 0x73,
		0x74, 0x6f,
		0x6d, 0x2d,
		0x68, 0x65,
		0x61, 0x64,
		0x65, 0x72,
	}

	frame := NewStringLiteral(indexing, name, value)
	actual := frame.Encode()

	assert.Equal(t, actual.Bytes(), buf)
}

func TestIndexedLiteralEncode(t *testing.T) {
	// D.2.2.  Literal Header Field without Indexing
	var indexing Indexing = WITHOUT
	var index uint32 = 4
	var value string = "/sample/path"
	buf := []byte{
		0x04, 0x0c,
		0x2f, 0x73,
		0x61, 0x6d,
		0x70, 0x6c,
		0x65, 0x2f,
		0x70, 0x61,
		0x74, 0x68,
	}

	frame := NewIndexedLiteral(indexing, index, value)
	actual := frame.Encode()
	assert.Equal(t, actual.Bytes(), buf)
}

func TestIndexedHeaderEncode(t *testing.T) {
	// D.2.3.  Indexed Header Field
	var index uint32 = 2
	buf := []byte{0x82}

	frame := NewIndexedHeader(index)
	actual := frame.Encode()
	assert.Equal(t, actual.Bytes(), buf)
}
