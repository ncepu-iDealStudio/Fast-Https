package hpack

import (
	"fmt"
	"log"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

// The dynamic table is a table that associates stored header
// fields with index values.
// This table is dynamic and specific to an encoding or decoding context.
type DynamicTable struct {
	DYNAMIC_TABLE_SIZE uint32
	HeaderFields       []*HeaderField
}

func NewDynamicTable(SETTINGS_HEADER_TABLE_SIZE uint32) *DynamicTable {
	return &DynamicTable{
		SETTINGS_HEADER_TABLE_SIZE,
		make([]*HeaderField, 0),
	}
}

// get total size of Dynamic Table
func (ht *DynamicTable) Size() uint32 {
	var size uint32
	for _, h := range ht.HeaderFields {
		size += h.Size()
	}
	return size
}

// get length of Dynamic Table
func (ht *DynamicTable) Len() int {
	return len(ht.HeaderFields)
}

// push new Header Field to top of DynamicTable
// with eviction
func (ht *DynamicTable) Push(hf *HeaderField) {
	tmp := []*HeaderField{hf}
	ht.HeaderFields = append(tmp, ht.HeaderFields...)
}

// remove Header at index i
func (ht *DynamicTable) Remove(index int) *HeaderField {
	// https://code.google.com/p/go-wiki/wiki/SliceTricks
	copy(ht.HeaderFields[index:], ht.HeaderFields[index+1:])
	// avoid memory leak
	removed := ht.HeaderFields[len(ht.HeaderFields)-1]
	ht.HeaderFields[len(ht.HeaderFields)-1] = new(HeaderField) // GC
	ht.HeaderFields = ht.HeaderFields[:len(ht.HeaderFields)-1]
	return removed
}

// String for Debug
func (ht *DynamicTable) String() (str string) {
	size := ht.Size()
	max := ht.DYNAMIC_TABLE_SIZE
	str += fmt.Sprintf("\n------ HT(%v/%v) ------\n", size, max)
	for i, v := range ht.HeaderFields {
		str += fmt.Sprintln(i, v)
	}
	str += "---------------------\n"
	return str
}
