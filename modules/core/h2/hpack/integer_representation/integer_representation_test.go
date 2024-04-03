package integer_representation

import (
	assert "github.com/Jxck/assertion"
	"github.com/Jxck/swrap"
	"testing"
	"testing/quick"
)

func TestEncode(t *testing.T) {
	testcases := []struct {
		actual   swrap.SWrap
		expected []byte
	}{
		{Encode(10, 5), []byte{10}},
		{Encode(40, 5), []byte{31, 9}},
		{Encode(42, 0), []byte{42}},
		{Encode(1337, 5), []byte{31, 154, 10}},
		{Encode(3000000, 5), []byte{31, 161, 141, 183, 1}},
	}

	for _, testcase := range testcases {
		actual := testcase.actual.Bytes()
		expected := testcase.expected
		assert.Equal(t, actual, expected)
	}
}

func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(10, 5)
		Encode(40, 5)
		Encode(42, 0)
		Encode(1337, 5)
		Encode(3000000, 5)
	}
}

func TestDecode(t *testing.T) {
	testcases := []struct {
		expected, actual uint32
	}{
		{Decode(swrap.New([]byte{10}), 5), 10},
		{Decode(swrap.New([]byte{31, 9}), 5), 40},
		{Decode(swrap.New([]byte{31, 154, 10}), 5), 1337},
		{Decode(swrap.New([]byte{31, 161, 141, 183, 1}), 5), 3000000},
	}

	for _, testcase := range testcases {
		actual := testcase.actual
		expected := testcase.expected
		assert.Equal(t, actual, expected)
	}
}

func BenchmarkDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Decode(swrap.New([]byte{10}), 5)
		Decode(swrap.New([]byte{31, 9}), 5)
		Decode(swrap.New([]byte{31, 154, 10}), 5)
		Decode(swrap.New([]byte{31, 161, 141, 183, 1}), 5)
	}
}

func TestEncodeDecodeQuickCheck(t *testing.T) {
	f := func(I uint32) bool {
		var N uint8 = 5
		buf := Encode(I, N)
		actual := Decode(buf, N)
		t.Log(I)
		t.Log(actual)
		return actual == I
	}
	c := new(quick.Config)

	if err := quick.Check(f, c); err != nil {
		t.Error(err)
	}
}

func TestReadPrefixedInteger(t *testing.T) {
	// 0x1F 0001 1111
	// 0x95 1001 0101
	// 0x0A 0000 1010
	// 0x06 0000 0110
	var prefix uint8 = 5
	buf := swrap.New([]byte{0x1F, 0x95, 0x0A, 0x06})
	expected := []byte{0x1F, 0x95, 0xA}
	actual := ReadPrefixedInteger(&buf, prefix)
	assert.Equal(t, actual.Bytes(), expected)
}
