package huffman

import (
	"fmt"
	"log"
)

func init() {
	log.SetFlags(log.Lshortfile)
}

// 結果 1 byte を生成するための struct
type byt struct {
	value  uint32
	remain byte
}

func newByt() *byt {
	return &byt{0, 8}
}

func (b *byt) String() string {
	return fmt.Sprintf("(v=%v, r=%v)", b.value, b.remain)
}

func Encode(raw []byte) (encoded []byte) {
	table := huffmanTable
	// 空のバイト列はそのまま返す
	if len(raw) == 0 {
		return raw
	}

	// 1 byte の入れ物
	b := newByt()

	for _, v := range raw {

		// huffman table で変換
		huff := table[v]

		for huff.length > 0 { // huff.code を使いきるまで

			if b.remain > huff.length {
				// huff.code の全てを入れる

				// 左シフトして桁を合わせる
				shift := b.remain - huff.length
				tmp := huff.code << shift

				// byt に追加、入れ多分だけ長さを減らす
				b.value += tmp
				b.remain = shift

				// huff は空に
				huff.length = 0

			} else {
				// huff.code の一部を入れる

				// 右シフトして入れる分だけ切り出す
				shift := huff.length - b.remain
				tmp := huff.code >> shift

				// byt に追加、もう入らない
				b.value += tmp
				b.remain = 0

				// huff から使った分だけ減らす
				huff.code -= (tmp << shift)
				huff.length = shift
			}

			if b.remain == 0 {
				// byt が埋まったら配列に移して初期化
				encoded = append(encoded, byte(b.value))
				b = newByt()
			}
		}
	}

	if b.remain > 0 && b.remain < 8 { // EOS でパディング
		// パディング分切り出す
		eos := huffmanTable[256]
		shift := eos.length - b.remain
		padding := eos.code >> shift

		// byt に加える
		b.value += padding
		b.remain = 0

		// 配列に移す
		encoded = append(encoded, byte(b.value))
		// 最後なのでゼロ値でGC
		b = nil
	}

	return encoded
}
