package hpack

import (
	"github.com/Jxck/hpack/huffman"
	integer "github.com/Jxck/hpack/integer_representation"
	. "github.com/Jxck/logger"
	"github.com/Jxck/swrap"
)

func (frame *IndexedHeader) Encode() (buf *swrap.SWrap) {
	buf = swrap.Make(integer.Encode(frame.Index, 7))
	(*buf)[0] += 0x80
	if frame.Index == 0 {
		// TODO: Encoding Error
	}
	return buf
}

func (frame *IndexedLiteral) Encode() (buf *swrap.SWrap) {
	switch frame.Indexing {
	case WITH:
		buf = swrap.Make(integer.Encode(frame.Index, 6))
		(*buf)[0] += 0x20
	case WITHOUT:
		buf = swrap.Make(integer.Encode(frame.Index, 4))
	case NEVER:
		buf = swrap.Make(integer.Encode(frame.Index, 4))
		(*buf)[0] += 0x8
	}

	// No Huffman
	buf.Merge(integer.Encode(frame.ValueLength, 7))
	buf.Merge([]byte(frame.ValueString))

	return buf
}

func (frame *IndexedLiteral) EncodeHuffman() (buf *swrap.SWrap) {
	switch frame.Indexing {
	case WITH:
		buf = swrap.Make(integer.Encode(frame.Index, 6))
		(*buf)[0] += 0x20
	case WITHOUT:
		buf = swrap.Make(integer.Encode(frame.Index, 4))
	case NEVER:
		buf = swrap.Make(integer.Encode(frame.Index, 4))
		(*buf)[0] += 0x8
	}

	var encoded, length []byte

	// Value With Huffman
	encoded = huffman.Encode([]byte(frame.ValueString))
	length = integer.Encode(uint32(len(encoded)), 7)
	length[0] += 0x80 // 1000 0000 (huffman flag)
	buf.Merge(length)
	buf.Merge(encoded)

	return buf
}

func (frame *StringLiteral) Encode() (buf *swrap.SWrap) {
	buf = new(swrap.SWrap)
	switch frame.Indexing {
	case WITH:
		buf.Add(0x40) // 0100 0000
	case WITHOUT:
		buf.Add(0) // 0000 0000
	case NEVER:
		buf.Add(0x10) // 0001 0000
	}

	// No Huffman
	buf.Merge(integer.Encode(frame.NameLength, 7))
	buf.Merge([]byte(frame.NameString))

	// No Huffman
	buf.Merge(integer.Encode(frame.ValueLength, 7))
	buf.Merge([]byte(frame.ValueString))

	return buf
}

func (frame *StringLiteral) EncodeHuffman() (buf *swrap.SWrap) {
	buf = new(swrap.SWrap)
	switch frame.Indexing {
	case WITH:
		buf.Add(0x40) // 0100 0000
	case WITHOUT:
		buf.Add(0) // 0000 0000
	case NEVER:
		buf.Add(0x10) // 0001 0000
	}

	var encoded, length []byte

	// Name With Huffman
	encoded = huffman.Encode([]byte(frame.NameString))
	length = integer.Encode(uint32(len(encoded)), 7)
	length[0] += 0x80 // 1000 0000 (huffman flag)
	buf.Merge(length)
	buf.Merge(encoded)

	// Value With Huffman
	encoded = huffman.Encode([]byte(frame.ValueString))
	length = integer.Encode(uint32(len(encoded)), 7)
	length[0] += 0x80 // 1000 0000 (huffman flag)
	buf.Merge(length)
	buf.Merge(encoded)

	Trace("huffman encoded %s %v", frame, *buf)
	return buf
}

func (frame *DynamicTableSizeUpdate) Encode() (buf *swrap.SWrap) {
	buf = new(swrap.SWrap)
	buf.Add(0x20) // 0010 0000
	buf.Merge(integer.Encode(frame.MaxSize, 4))
	return buf
}
