package hpack

import (
	"fmt"
	"github.com/Jxck/swrap"
)

const Version int = 9

type Indexing int

const (
	WITH Indexing = iota
	WITHOUT
	NEVER
)

func (i Indexing) String() string {
	return []string{
		"WITH",
		"WITHOUT",
		"NEVER",
	}[i]
}

type Frame interface {
	Encode() *swrap.SWrap
}

// Indexed Header Field
//
// 	0   1   2   3   4   5   6   7
// +---+---+---+---+---+---+---+---+
// | 1 |        Index (7+)         |
// +---+---------------------------+
type IndexedHeader struct {
	Index uint32
}

func NewIndexedHeader(index uint32) (frame *IndexedHeader) {
	frame = new(IndexedHeader)
	frame.Index = index
	return
}

// Literal Header Field with Incremental Indexing - Indexed Name
// | 0 | 1 |      Index (6+)       |
//
// Literal Header Field without Indexing - Indexed Name
// | 0 | 0 | 0 | 0 |  Index (4+)   |
//
// Literal Header Field never Indexed - Indexed Nmae
// | 0 | 0 | 0 | 1 |  Index (4+)   |
//
//  0   1   2   3   4   5   6   7
// +---+---+---+---+---+---+---+---+
// | 0 |       Flag + Index        |
// +---+---+---+-------------------+
// | H |     Value Length (7+)     |
// +-------------------------------+
// | Value String (Length octets)  |
// +-------------------------------+
type IndexedLiteral struct {
	Indexing    Indexing
	Index       uint32
	ValueLength uint32
	ValueString string
}

func NewIndexedLiteral(indexing Indexing, index uint32, value string) (frame *IndexedLiteral) {
	frame = new(IndexedLiteral)
	frame.Indexing = indexing
	frame.Index = index
	frame.ValueLength = uint32(len(value))
	frame.ValueString = value
	return
}

func (f IndexedLiteral) String() string {
	str := fmt.Sprintf("IL) %s {%d, %s(%d)}",
		f.Indexing,
		f.Index,
		f.ValueString, f.ValueLength,
	)
	return str
}

// Literal Header Field with Incremental Indexing - New Name
// | 0 | 1 |           0           | = 64
//
// Literal Header Field without Indexing - New Name
// |               0               | =  0
//
// Literal Header Field never Indexed - New Name
// |     0     | 1 |       0       | = 16
//
//   0   1   2   3   4   5   6   7
// +---+---+---+---+---+---+---+---+
// |     Flag=( 0 | 16 | 64 )      |
// +---+---+---+-------------------+
// | H |     Name Length (7+)      |
// +-------------------------------+
// |  Name String (Length octets)  |
// +-------------------------------+
// | H |     Value Length (7+)     |
// +-------------------------------+
// | Value String (Length octets)  |
// +-------------------------------+
type StringLiteral struct {
	Indexing    Indexing
	Index       uint32
	NameLength  uint32
	NameString  string
	ValueLength uint32
	ValueString string
}

func NewStringLiteral(indexing Indexing, name, value string) (frame *StringLiteral) {
	frame = new(StringLiteral)
	frame.Indexing = indexing
	frame.NameLength = uint32(len(name))
	frame.NameString = name
	frame.ValueLength = uint32(len(value))
	frame.ValueString = value
	return
}

func (f StringLiteral) String() string {
	str := fmt.Sprintf("SL) %s {%s(%d), %s(%d)}",
		f.Indexing,
		f.NameString, f.NameLength,
		f.ValueString, f.ValueLength,
	)
	return str
}

// 0   1   2   3   4   5   6   7
// +---+---+---+---+---+---+---+---+
// | 0 | 0 | 1 |   Max size (5+)   |
// +---+---------------------------+
// Maximum Dynamic Table Size Update
type DynamicTableSizeUpdate struct {
	MaxSize uint32
}

func NewDynamicTableSizeUpdate(maxSize uint32) (frame *DynamicTableSizeUpdate) {
	frame = new(DynamicTableSizeUpdate)
	frame.MaxSize = maxSize
	return
}

func (f DynamicTableSizeUpdate) String() string {
	str := fmt.Sprintf("Header Table Size Change to: %d", f.MaxSize)
	return str
}
