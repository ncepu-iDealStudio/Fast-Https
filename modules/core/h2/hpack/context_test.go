package hpack

import (
	assert "github.com/Jxck/assertion"
	. "github.com/Jxck/color"
	. "github.com/Jxck/logger"
	"sort"
	"testing"
)

const DEFAULT_HEADER_TABLE_SIZE uint32 = 4096

func TestRequestWithoutHuffman(t *testing.T) {
	// D.3.  Request Examples without Huffman
	var (
		context    *Context
		buf        []byte
		expectedES *HeaderList
		expectedHT *DynamicTable
	)

	context = NewContext(DEFAULT_HEADER_TABLE_SIZE)

	/**
	 * D.3.1.  First request
	 */
	Debug(Pink("\n========== First Request ==============="))

	buf = []byte{
		0x82, 0x86,
		0x84, 0x41,
		0x0f, 0x77,
		0x77, 0x77,
		0x2e, 0x65,
		0x78, 0x61,
		0x6d, 0x70,
		0x6c, 0x65,
		0x2e, 0x63,
		0x6f, 0x6d,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":path", "/"),
		NewHeaderField(":authority", "www.example.com"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.3.2.  Second request
	 */
	Debug(Pink("\n========== Second Request ==============="))

	buf = []byte{
		0x82, 0x86,
		0x84, 0xbe,
		0x58, 0x08,
		0x6e, 0x6f,
		0x2d, 0x63,
		0x61, 0x63,
		0x68, 0x65,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":path", "/"),
		NewHeaderField(":authority", "www.example.com"),
		NewHeaderField("cache-control", "no-cache"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("cache-control", "no-cache"),
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.3.3.  Third request
	 */
	Debug(Pink("\n========== Third Request ==============="))

	buf = []byte{
		0x82, 0x87,
		0x85, 0xbf,
		0x40, 0x0a,
		0x63, 0x75,
		0x73, 0x74,
		0x6f, 0x6d,
		0x2d, 0x6b,
		0x65, 0x79,
		0x0c, 0x63,
		0x75, 0x73,
		0x74, 0x6f,
		0x6d, 0x2d,
		0x76, 0x61,
		0x6c, 0x75,
		0x65,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "https"),
		NewHeaderField(":path", "/index.html"),
		NewHeaderField(":authority", "www.example.com"),
		NewHeaderField("custom-key", "custom-value"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("custom-key", "custom-value"),
		NewHeaderField("cache-control", "no-cache"),
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)
}

func TestRequestWithHuffman(t *testing.T) {
	// D.4.  Request Examples with Huffman
	var (
		context    *Context
		buf        []byte
		expectedES *HeaderList
		expectedHT *DynamicTable
	)

	context = NewContext(DEFAULT_HEADER_TABLE_SIZE)

	/**
	 * D.4.1.  First request
	 */
	Debug(Pink("\n========== First Request ==============="))

	buf = []byte{
		0x82, 0x86,
		0x84, 0x41,
		0x8c, 0xf1,
		0xe3, 0xc2,
		0xe5, 0xf2,
		0x3a, 0x6b,
		0xa0, 0xab,
		0x90, 0xf4,
		0xff,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":path", "/"),
		NewHeaderField(":authority", "www.example.com"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.4.2.  Second request
	 */
	Debug(Pink("\n========== Second Request ==============="))

	buf = []byte{
		0x82, 0x86,
		0x84, 0xbe,
		0x58, 0x86,
		0xa8, 0xeb,
		0x10, 0x64,
		0x9c, 0xbf,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":path", "/"),
		NewHeaderField(":authority", "www.example.com"),
		NewHeaderField("cache-control", "no-cache"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("cache-control", "no-cache"),
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.4.3.  Third request
	 */
	Debug(Pink("\n========== Third Request ==============="))

	buf = []byte{
		0x82, 0x87,
		0x85, 0xbf,
		0x40, 0x88,
		0x25, 0xa8,
		0x49, 0xe9,
		0x5b, 0xa9,
		0x7d, 0x7f,
		0x89, 0x25,
		0xa8, 0x49,
		0xe9, 0x5b,
		0xb8, 0xe8,
		0xb4, 0xbf,
	}

	expectedES = &HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "https"),
		NewHeaderField(":path", "/index.html"),
		NewHeaderField(":authority", "www.example.com"),
		NewHeaderField("custom-key", "custom-value"),
	}

	expectedHT = NewDynamicTable(DEFAULT_HEADER_TABLE_SIZE)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("custom-key", "custom-value"),
		NewHeaderField("cache-control", "no-cache"),
		NewHeaderField(":authority", "www.example.com"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)
}

func TestResponseWithoutHuffman(t *testing.T) {
	// D.5.  Response Examples without Huffman
	var (
		context    *Context
		buf        []byte
		expectedES *HeaderList
		expectedHT *DynamicTable
	)

	var DynamicTableSize uint32 = 256
	context = NewContext(DynamicTableSize)

	/**
	 * D.5.1.  First response
	 */
	Debug(Pink("\n========== First Response ==============="))

	buf = []byte{
		0x48, 0x03,
		0x33, 0x30,
		0x32, 0x58,
		0x07, 0x70,
		0x72, 0x69,
		0x76, 0x61,
		0x74, 0x65,
		0x61, 0x1d,
		0x4d, 0x6f,
		0x6e, 0x2c,
		0x20, 0x32,
		0x31, 0x20,
		0x4f, 0x63,
		0x74, 0x20,
		0x32, 0x30,
		0x31, 0x33,
		0x20, 0x32,
		0x30, 0x3a,
		0x31, 0x33,
		0x3a, 0x32,
		0x31, 0x20,
		0x47, 0x4d,
		0x54, 0x6e,
		0x17, 0x68,
		0x74, 0x74,
		0x70, 0x73,
		0x3a, 0x2f,
		0x2f, 0x77,
		0x77, 0x77,
		0x2e, 0x65,
		0x78, 0x61,
		0x6d, 0x70,
		0x6c, 0x65,
		0x2e, 0x63,
		0x6f, 0x6d,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "302"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("location", "https://www.example.com"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField(":status", "302"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.5.2.  Second response
	 */
	Debug(Pink("\n========== Second Response ==============="))

	buf = []byte{
		0x48, 0x03,
		0x33, 0x30,
		0x37, 0xc1,
		0xc0, 0xbf,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "307"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("location", "https://www.example.com"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField(":status", "307"),
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("cache-control", "private"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.5.3.  Third response
	 */
	Debug(Pink("\n========== Third Response ==============="))

	buf = []byte{
		0x88, 0xc1,
		0x61, 0x1d,
		0x4d, 0x6f,
		0x6e, 0x2c,
		0x20, 0x32,
		0x31, 0x20,
		0x4f, 0x63,
		0x74, 0x20,
		0x32, 0x30,
		0x31, 0x33,
		0x20, 0x32,
		0x30, 0x3a,
		0x31, 0x33,
		0x3a, 0x32,
		0x32, 0x20,
		0x47, 0x4d,
		0x54, 0xc0,
		0x5a, 0x04,
		0x67, 0x7a,
		0x69, 0x70,
		0x77, 0x38,
		0x66, 0x6f,
		0x6f, 0x3d,
		0x41, 0x53,
		0x44, 0x4a,
		0x4b, 0x48,
		0x51, 0x4b,
		0x42, 0x5a,
		0x58, 0x4f,
		0x51, 0x57,
		0x45, 0x4f,
		0x50, 0x49,
		0x55, 0x41,
		0x58, 0x51,
		0x57, 0x45,
		0x4f, 0x49,
		0x55, 0x3b,
		0x20, 0x6d,
		0x61, 0x78,
		0x2d, 0x61,
		0x67, 0x65,
		0x3d, 0x33,
		0x36, 0x30,
		0x30, 0x3b,
		0x20, 0x76,
		0x65, 0x72,
		0x73, 0x69,
		0x6f, 0x6e,
		0x3d, 0x31,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "200"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:22 GMT"),
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("content-encoding", "gzip"),
		NewHeaderField("set-cookie", "foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("set-cookie", "foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1"),
		NewHeaderField("content-encoding", "gzip"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:22 GMT"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)
}

func TestResponseWithHuffman(t *testing.T) {
	// D.6.  Response Examples with Huffman
	var (
		context    *Context
		buf        []byte
		expectedES *HeaderList
		expectedHT *DynamicTable
	)

	var DynamicTableSize uint32 = 256
	context = NewContext(DynamicTableSize)

	/**
	 * D.6.1.  First response
	 */
	Debug(Pink("\n========== First Response ==============="))

	buf = []byte{
		0x48, 0x82,
		0x64, 0x02,
		0x58, 0x85,
		0xae, 0xc3,
		0x77, 0x1a,
		0x4b, 0x61,
		0x96, 0xd0,
		0x7a, 0xbe,
		0x94, 0x10,
		0x54, 0xd4,
		0x44, 0xa8,
		0x20, 0x05,
		0x95, 0x04,
		0x0b, 0x81,
		0x66, 0xe0,
		0x82, 0xa6,
		0x2d, 0x1b,
		0xff, 0x6e,
		0x91, 0x9d,
		0x29, 0xad,
		0x17, 0x18,
		0x63, 0xc7,
		0x8f, 0x0b,
		0x97, 0xc8,
		0xe9, 0xae,
		0x82, 0xae,
		0x43, 0xd3,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "302"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("location", "https://www.example.com"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField(":status", "302"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.6.2.  Second response
	 */
	Debug(Pink("\n========== Second Response ==============="))

	buf = []byte{
		0x48, 0x83,
		0x64, 0x0e,
		0xff, 0xc1,
		0xc0, 0xbf,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "307"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("location", "https://www.example.com"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField(":status", "307"),
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:21 GMT"),
		NewHeaderField("cache-control", "private"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)

	/**
	 * D.6.3.  Third response
	 */
	Debug(Pink("\n========== Third Response ==============="))

	buf = []byte{
		0x88, 0xc1,
		0x61, 0x96,
		0xd0, 0x7a,
		0xbe, 0x94,
		0x10, 0x54,
		0xd4, 0x44,
		0xa8, 0x20,
		0x05, 0x95,
		0x04, 0x0b,
		0x81, 0x66,
		0xe0, 0x84,
		0xa6, 0x2d,
		0x1b, 0xff,
		0xc0, 0x5a,
		0x83, 0x9b,
		0xd9, 0xab,
		0x77, 0xad,
		0x94, 0xe7,
		0x82, 0x1d,
		0xd7, 0xf2,
		0xe6, 0xc7,
		0xb3, 0x35,
		0xdf, 0xdf,
		0xcd, 0x5b,
		0x39, 0x60,
		0xd5, 0xaf,
		0x27, 0x08,
		0x7f, 0x36,
		0x72, 0xc1,
		0xab, 0x27,
		0x0f, 0xb5,
		0x29, 0x1f,
		0x95, 0x87,
		0x31, 0x60,
		0x65, 0xc0,
		0x03, 0xed,
		0x4e, 0xe5,
		0xb1, 0x06,
		0x3d, 0x50,
		0x07,
	}

	expectedES = &HeaderList{
		NewHeaderField(":status", "200"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:22 GMT"),
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("content-encoding", "gzip"),
		NewHeaderField("set-cookie", "foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1"),
	}

	expectedHT = NewDynamicTable(DynamicTableSize)
	expectedHT.HeaderFields = []*HeaderField{
		NewHeaderField("set-cookie", "foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1"),
		NewHeaderField("content-encoding", "gzip"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:22 GMT"),
	}

	context.Decode(buf)

	// test Header Table
	assert.Equal(t, context.HT, expectedHT)

	// test Emitted Set
	sort.Sort(context.ES)
	sort.Sort(expectedES)
	assert.Equal(t, context.ES, expectedES)
}

func TestEncodeDecode(t *testing.T) {
	context := NewContext(DEFAULT_HEADER_TABLE_SIZE)

	hl := HeaderList{
		NewHeaderField(":status", "200"),
		NewHeaderField("cache-control", "private"),
		NewHeaderField("date", "Mon, 21 Oct 2013 20:13:22 GMT"),
		NewHeaderField("location", "https://www.example.com"),
		NewHeaderField("content-encoding", "gzip"),
		NewHeaderField("set-cookie", "foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1"),
	}

	encoded := context.Encode(hl)
	context.Decode(encoded)
	assert.Equal(t, hl, *context.ES)
}

func TestEncodeDecodeWithHuffman(t *testing.T) {
	context := NewContext(DEFAULT_HEADER_TABLE_SIZE)
	hl := HeaderList{
		NewHeaderField(":method", "GET"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":authority", "g-ecx.images-amazon.com"),
		NewHeaderField(":path", "/images/G/01/gno/beacon/BeaconSprite-US-01._V401903535_.png"),
		NewHeaderField("user-agent", "Mozilla/5.0 NewHeaderField(Macintosh; Intel Mac OS X 10.8; rv:16.0) Gecko/20100101 Firefox/16.0"),
		NewHeaderField("accept", "image/png,image/*;q=0.8,*/*;q=0.5"),
		NewHeaderField("accept-language", "en-US,en;q=0.5"),
		NewHeaderField("accept-encoding", "gzip, deflate"),
		NewHeaderField("connection", "keep-alive"),
		NewHeaderField("referer", "http://www.amazon.com/"),
	}

	encoded := context.Encode(hl)
	if encoded[len(encoded)-1] == 255 {
		t.Error("8bit EOS on huffman encoded result is error")
	}
	context.Decode(encoded)
	assert.Equal(t, hl, *context.ES)
}
