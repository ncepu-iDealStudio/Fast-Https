package hpack

import (
	"fmt"
	"net/http"
	"strings"
)

// A header list is an ordered collection of header fields that are encoded jointly
// and can contain duplicate header fields.
// A complete list of header fields contained in an HTTP/2 header block is a header list.
type HeaderList []*HeaderField

func NewHeaderList() *HeaderList {
	return new(HeaderList)
}

func ToHeaderList(header http.Header) *HeaderList {
	normal := *new(HeaderList)
	pseuds := *new(HeaderList)
	for key, values := range header {
		key := strings.ToLower(key)
		if strings.HasPrefix(key, ":") {
			// Pseudo Header Fields
			pseuds = append(pseuds, NewHeaderField(key, values[0]))
		} else {
			// Normal Header Fields
			for _, value := range values {
				normal = append(normal, NewHeaderField(key, value))
			}
		}
	}
	hl := append(pseuds, normal...)
	return &hl
}

func (hl *HeaderList) Emit(hf *HeaderField) {
	*hl = append(*hl, hf)
}

func (hl *HeaderList) Len() int {
	return len(*hl)
}

// Sort Interface
func (hl *HeaderList) Swap(i, j int) {
	h := *hl
	h[i], h[j] = h[j], h[i]
}

func (hl *HeaderList) Less(i, j int) bool {
	h := *hl
	if h[i].Name == h[j].Name {
		return h[i].Value < h[j].Value
	}
	return h[i].Name < h[j].Name
}

// convert to http.Header
func (hl HeaderList) ToHeader() http.Header {
	header := make(http.Header)
	for _, hf := range hl {
		header.Add(hf.Name, hf.Value)
	}
	return header
}

func (hl HeaderList) String() (str string) {
	str += "\n--------- HL ---------\n"
	for i, v := range hl {
		str += fmt.Sprintln(i, v)
	}
	str += "\n----------------------\n"
	return str
}
