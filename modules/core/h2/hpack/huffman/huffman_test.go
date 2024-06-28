package huffman

import (
	"fmt"
	assert "github.com/Jxck/assertion"
	"log"
	"reflect"
	"strconv"
	"testing"
	"testing/quick"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

func toHexString(hex []byte) (hexstr string) {
	for _, v := range hex {
		s := fmt.Sprintf("%x", v)
		if len(s) < 2 {
			s = "0" + s
		}
		hexstr += s
	}
	return hexstr
}

func toHexBytes(hexstr string) (hexbytes []byte) {
	for len(hexstr) > 0 {
		hex := hexstr[:2]
		b, _ := strconv.ParseInt(hex, 16, 64)
		hexbytes = append(hexbytes, byte(b))
		hexstr = hexstr[2:]
	}
	return hexbytes
}

var testCase = []struct {
	str, hex string
}{
	{"www.example.com", "f1e3c2e5f23a6ba0ab90f4ff"},
	{"no-cache", "a8eb10649cbf"},
	{"custom-key", "25a849e95ba97d7f"},
	{"custom-value", "25a849e95bb8e8b4bf"},
	{"302", "6402"},
	{"private", "aec3771a4b"},
	{
		"Mon, 21 Oct 2013 20:13:21 GMT",
		"d07abe941054d444a8200595040b8166e082a62d1bff",
	},
	{
		"https://www.example.com",
		"9d29ad171863c78f0b97c8e9ae82ae43d3",
	},
	{
		"307",
		"640eff",
	},
	{
		"Mon, 21 Oct 2013 20:13:22 GMT",
		"d07abe941054d444a8200595040b8166e084a62d1bff",
	},
	{
		"foo=ASDJKHQKBZXOQWEOPIUAXQWEOIU; max-age=3600; version=1",
		"94e7821dd7f2e6c7b335dfdfcd5b3960d5af27087f3672c1ab270fb5291f9587316065c003ed4ee5b1063d5007",
	},
}

func TestEncode(t *testing.T) {
	for _, tc := range testCase {
		raw := []byte(tc.str)
		expected := tc.hex
		encoded := Encode(raw)
		actual := toHexString(encoded)
		assert.Equal(t, actual, expected)
	}
}

func TestDecode(t *testing.T) {
	for _, tc := range testCase {
		expected := tc.str
		code := toHexBytes(tc.hex)
		actual := string(Decode(code))
		assert.Equal(t, actual, expected)
	}
}

func TestEncodeDecode(t *testing.T) {
	for _, tc := range testCase {
		expected := []byte(tc.str)
		encoded := Encode(expected)
		actual := Decode(encoded)
		assert.Equal(t, actual, expected)
	}
}

// TODO: fixme
func TestQuickCheckEncodeDecode(t *testing.T) {
	f := func(expected []byte) bool {
		var encoded, actual []byte
		encoded = Encode(expected)
		actual = Decode(encoded)
		return reflect.DeepEqual(actual, expected)
	}

	c := &quick.Config{
		MaxCountScale: 1000,
	}

	if err := quick.Check(f, c); err != nil {
		t.Error(err)
	}
}
