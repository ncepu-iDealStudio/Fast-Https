package compress

import (
	"fast-https/modules/compress"
	"fmt"
	"testing"
)

func TestCsGzip(t *testing.T) {
	input := []byte("Hello, World!")
	fmt.Println(input)

	compressed, err := compress.CompressBytesGzip(input)
	if err != nil {
		fmt.Println("error: ", err)
		return
	}

	fmt.Println("data after compress: ", compressed)

	mm, _ := compress.DecompressBytesGzip(compressed)
	fmt.Println(mm)
}

func TestCsBr(t *testing.T) {
	input := []byte("Hello, World!")
	fmt.Println(input)

	compressed, err := compress.CompressBytesBr(input)
	if err != nil {
		fmt.Println("error: ", err)
		return
	}

	fmt.Println("data after compress: ", compressed)

	mm, _ := compress.DecompressBytesBr(compressed)
	fmt.Println(mm)
}
