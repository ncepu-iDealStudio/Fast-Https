package hpack

import (
	assert "github.com/Jxck/assertion"
	"testing"
)

func TestHeaderFieldSize(t *testing.T) {
	h := NewHeaderField("hello", "world")
	var actual uint32 = 42
	assert.Equal(t, h.Size(), actual)
}
