package hpack

import (
	"github.com/Jxck/hpack/huffman"
	integer "github.com/Jxck/hpack/integer_representation"
	. "github.com/Jxck/logger"
	"github.com/Jxck/swrap"
	"log"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

// Decode Wire byte seq to Slice of Frames
func Decode(wire []byte) (frames []Frame) {
	buf := swrap.Make(wire)
	for buf.Len() > 0 {
		frames = append(frames, DecodeHeader(buf))
	}
	return frames
}

// Decode single Frame from buffer and return it
func DecodeHeader(buf *swrap.SWrap) Frame {
	// check first byte
	types := (*buf)[0]
	Trace("types = %v", types)
	if types >= 0x80 { // 1xxx xxxx
		Debug("Indexed Header Representation")

		index := DecodePrefixedInteger(buf, 7)
		Trace("Indexed = %v", index)
		frame := NewIndexedHeader(index)

		if index == 0 {
			// TODO: Decoding Error
			// log.Fatal("Decoding Error: The index value of 0 is not used.")
		}
		return frame
	}
	if types == 0 { // 0000 0000
		Debug("StringLiteral (indexing = WITHOUT)")

		// remove first byte defines type
		buf.Shift()

		indexing := WITHOUT
		name := DecodeLiteral(buf)
		Trace("StringLiteral name = %v", name)
		value := DecodeLiteral(buf)
		Trace("StringLiteral value = %v", value)
		frame := NewStringLiteral(indexing, name, value)
		return frame
	}
	if types == 0x10 { // 0001 0000
		Debug("StringLiteral (indexing = NEVER)")

		// remove first byte defines type
		buf.Shift()

		indexing := NEVER
		name := DecodeLiteral(buf)
		Trace("StringLiteral name = %v", name)
		value := DecodeLiteral(buf)
		Trace("StringLiteral value = %v", value)
		frame := NewStringLiteral(indexing, name, value)
		return frame
	}
	if types == 0x40 { // 0100 0000
		Debug("StringLiteral (indexing = WITH)")

		// remove first byte defines type
		buf.Shift()

		indexing := WITH
		name := DecodeLiteral(buf)
		Trace("StringLiteral name = %v", name)
		value := DecodeLiteral(buf)
		Trace("StringLiteral value = %v", value)
		frame := NewStringLiteral(indexing, name, value)
		return frame
	}
	if types&0xc0 == 0x40 { // 01xx xxxx & 1100 0000 == 0100 0000
		Debug("IndexedLiteral (indexing = WITH)")

		indexing := WITH
		index := DecodePrefixedInteger(buf, 6)
		Trace("IndexedLiteral index = %v", index)
		value := DecodeLiteral(buf)
		Trace("IndexedLiteral value = %v", value)
		frame := NewIndexedLiteral(indexing, index, value)
		return frame
	}
	if types&0xf0 == 0 { // 0000 xxxx & 1111 0000 == 0000 0000
		Debug("IndexedLiteral (indexing = WITHOUT)")

		indexing := WITHOUT
		index := DecodePrefixedInteger(buf, 4)
		Trace("IndexedLiteral index = %v", index)
		value := DecodeLiteral(buf)
		Trace("IndexedLiteral value = %v", value)
		frame := NewIndexedLiteral(indexing, index, value)
		return frame
	}
	if types&0xf0 == 0x10 { // 0000 xxxx & 1111 0000 == 0001 0000
		Debug("IndexedLiteral (indexing = NEVER)")

		indexing := NEVER
		index := DecodePrefixedInteger(buf, 4)
		Trace("IndexedLiteral index = %v", index)
		value := DecodeLiteral(buf)
		Trace("IndexedLiteral value = %v", value)
		frame := NewIndexedLiteral(indexing, index, value)
		return frame
	}
	if types&0xe0 == 0x20 { // 001x xxxx & 1110 0000 == 0010 0000
		Debug("Header Table Size Update")

		maxSize := DecodePrefixedInteger(buf, 5)
		frame := NewDynamicTableSizeUpdate(maxSize)
		return frame
	}
	return nil
}

// read N prefixed Integer from buffer as uint32
func DecodePrefixedInteger(buf *swrap.SWrap, N uint8) uint32 {
	tmp := integer.ReadPrefixedInteger(buf, N)
	return integer.Decode(tmp, N)
}

// read n byte from buffer as string
func DecodeString(buf *swrap.SWrap, n uint32) string {
	valueBytes := make([]byte, 0, n)
	for i := n; i > 0; i-- {
		valueBytes = append(valueBytes, buf.Shift())
	}
	return string(valueBytes)
}

func DecodeLiteral(buf *swrap.SWrap) (value string) {
	// 最初のバイトを取り出す
	first := (*buf)[0]

	// 最初の 1bit をみて huffman かどうか取得
	huffmanEncoded := (first&0x80 == 0x80)

	Trace("huffman = %t", huffmanEncoded)
	if huffmanEncoded {
		// 最初のバイトから 1 bit 目を消す
		(*buf)[0] = first & 127

		// ここで prefixed Integer 7 で読む。
		b := DecodePrefixedInteger(buf, 7)
		Trace("Literal Length = %v, buf size=%v", b, buf.Len())

		// その長さの分だけバイト値を取り出す
		code := make([]byte, 0)
		for ; b > 0; b-- {
			code = append(code, buf.Shift())
		}

		// ハフマンデコード
		value = string(huffman.Decode(code))
		Trace("decoded = %v", value)
	} else {
		valueLength := DecodePrefixedInteger(buf, 7)
		value = DecodeString(buf, valueLength)
	}
	return value
}
