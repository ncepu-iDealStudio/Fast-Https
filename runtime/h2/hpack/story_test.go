package hpack

import (
	"encoding/hex"
	"encoding/json"
	assert "github.com/Jxck/assertion"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"testing"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

// {
//   "draft": 5,
//   "description": "Encoded request headers with Literal without index only.",
//   "cases": [
//     {
//       "seqno": 0,
//       "header_table_size": 4096,
//       "wire": "1234567890abcdef",
//       "headers": [
//         { ":method": "GET" },
//         { ":scheme": "http" },
//         { ":authority": "example.com" },
//         { ":path": "/" },
//         { "x-my-header": "value1,value2" }
//       ]
//     },
//     .....
//   ]
// }
type TestCase struct {
	Seqno           int                 `json:"seqno"`
	HeaderTableSize uint32              `json:"header_table_size,omitempty"`
	Wire            string              `json:"wire"`
	Headers         []map[string]string `json:"headers"`
}

type TestFile struct {
	Draft       int        `json:"draft"`
	Description string     `json:"description"`
	Cases       []TestCase `json:"cases"`
}

func readJsonFile(path string) TestFile {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	var test TestFile
	err = json.NewDecoder(file).Decode(&test)
	if err != nil {
		log.Fatal(err)
	}
	return test
}

func RunStory(testfile TestFile, t *testing.T) {
	context := NewContext(DEFAULT_HEADER_TABLE_SIZE)
	for _, cases := range testfile.Cases {
		wire, err := hex.DecodeString(cases.Wire)
		if err != nil {
			log.Fatal(err)
		}
		context.Decode(wire)

		expectedES := new(HeaderList)
		for _, header := range cases.Headers {
			for key, value := range header {
				expectedES.Emit(NewHeaderField(key, value))
			}
		}

		sort.Sort(context.ES)
		sort.Sort(expectedES)
		assert.Equal(t, context.ES, expectedES)
	}
}

// func TestSingleStory(t *testing.T) {
// 	testcases := readJsonFile(dir + "story_29.json")
// 	RunStory(testcases, t)
// }

func TestStory(t *testing.T) {
	dirs := []string{
		"./hpack-test-case/go-hpack/",
		"./hpack-test-case/haskell-http2-linear-huffman/",
		"./hpack-test-case/haskell-http2-linear/",
		"./hpack-test-case/haskell-http2-naive-huffman/",
		"./hpack-test-case/haskell-http2-naive/",
		"./hpack-test-case/haskell-http2-static-huffman/",
		"./hpack-test-case/haskell-http2-static/",
		"./hpack-test-case/nghttp2-16384-4096/",
		"./hpack-test-case/nghttp2-change-table-size/",
		"./hpack-test-case/nghttp2/",
		"./hpack-test-case/node-http2-hpack/",
	}

	for _, dir := range dirs {
		files, _ := ioutil.ReadDir(dir)
		for _, f := range files {
			t.Log("==== test", dir+f.Name())
			testcases := readJsonFile(dir + f.Name())
			if testcases.Draft == Version {
				RunStory(testcases, t)
			}
		}
	}
}

func writeJson(src, dst, filename string) {
	testFile := readJsonFile(src + filename)

	testFile.Draft = Version
	testFile.Description = "" +
		"https://github.com/Jxck/hpack implemeted in Golang. " +
		"Encoded using String Literal with Huffman, " +
		"no Header/Static Table, " +
		"and always start with emptied Reference Set. by Jxck."

	context := NewContext(DEFAULT_HEADER_TABLE_SIZE)
	// 一つのケースごと
	for i, c := range testFile.Cases {
		hl := *new(HeaderList)
		// 一つのヘッダごと
		for _, header := range c.Headers {
			for key, value := range header {
				hl = append(hl, NewHeaderField(key, value))
			}
		}

		hexdump := context.Encode(hl)
		wire := hex.EncodeToString(hexdump)
		testFile.Cases[i].Seqno = i
		testFile.Cases[i].Wire = wire
		testFile.Cases[i].HeaderTableSize = DEFAULT_HEADER_TABLE_SIZE
	}

	b, _ := json.MarshalIndent(testFile, "", "  ")
	file, err := os.Create(dst + filename)
	if err != nil {
		log.Fatal(err)
	}
	n, err := file.Write(b)
	if n == 0 || err != nil {
		log.Fatal("failt to write file")
	}
}

func TestEncodeStory(t *testing.T) {
	src := "./hpack-test-case/raw-data/"
	dst := "./hpack-test-case/go-hpack/"
	files, _ := ioutil.ReadDir(src)
	for _, file := range files {
		writeJson(src, dst, file.Name())
	}
}
