package hpack

import (
	assert "github.com/Jxck/assertion"
	"testing"
)

func TestDynamicTableSizeLen(t *testing.T) {
	ht := DynamicTable{
		DEFAULT_HEADER_TABLE_SIZE,
		[]*HeaderField{
			NewHeaderField("1234", "1234"),
			NewHeaderField("1234", "1234"),
			NewHeaderField("1234", "1234"),
			NewHeaderField("1234", "1234"),
			NewHeaderField("1234", "1234"),
		},
	}

	var actual uint32 = 200
	assert.Equal(t, ht.Size(), actual)
	assert.Equal(t, ht.Len(), 5)
}
