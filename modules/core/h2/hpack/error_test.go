package hpack

import "testing"

func TestDecodeIndexZeroReturnsError(t *testing.T) {
	ctx := NewContext(DEFAULT_HEADER_TABLE_SIZE)
	if err := ctx.Decode([]byte{0x80}); err == nil {
		t.Fatal("index 0 should be a decoding error")
	}
}

func TestDecodeDynamicIndexOutOfRangeReturnsError(t *testing.T) {
	ctx := NewContext(DEFAULT_HEADER_TABLE_SIZE)
	// Indexed representation, index = static table size + 1, dynamic table empty.
	index := uint32(STATIC_HEADER_TABLE_SIZE + 1)
	wire := []byte{0x80 | byte(index)}
	if index >= 127 {
		t.Fatal("static table larger than the 7-bit prefix; update this fixture")
	}
	if err := ctx.Decode(wire); err == nil {
		t.Fatal("dynamic index past the table should be a decoding error")
	}
}

func TestEncodeIndexZeroReturnsNil(t *testing.T) {
	frame := NewIndexedHeader(0)
	if frame.Encode() != nil {
		t.Fatal("index 0 should not produce an encoded header")
	}
}
