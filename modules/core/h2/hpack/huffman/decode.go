package huffman

import (
	"fmt"
	"log"
)

var (
	HuffmanTree *node
)

func init() {
	log.SetFlags(log.Lshortfile)

	HuffmanTree = BuildTree(huffmanTable)
}

// huffman tree の node
type node struct {
	left, right *node // 左右の子ノードへのポインタ
	data        int   // Table のインデクスを持つ 257 なので int
}

// 空のノードを生成
// data int を持たないことを -1 で表わす
func newNode() *node {
	return &node{nil, nil, -1}
}

// デバッグ用
func (n *node) String() string {
	return fmt.Sprintf(
		"%p %p %p (%d)\n",
		n, n.left, n.right, n.data)
}

// デバッグ用
func show(current *node) {
	if current != nil {
		log.Println(current)
		show(current.left)
		show(current.right)
	}
}

// ツリーを構築する
func BuildTree(table *HuffmanTable) (root *node) {
	root = newNode()

	for i, huff := range table { // 全てのコードについて実施
		current := root // たどる時は根から

		for huff.length > 0 { // 1 コードの長さを消化するまで繰り返す
			huff.length -= 1
			mask := uint32(1 << huff.length) // length=5 なら 10000 でマスクする
			if huff.code&mask == mask {      // 11011 & 10000 = 10000 マスク結果とマスクの比較でわかる
				next := current.right // 1 だったら右
				if next == nil {      // 無かったらノードを足す
					next = newNode()
					current.right = next
				}
				current = next
			} else {
				next := current.left // 0 だったら左
				if next == nil {     // 無かったらノードを足す
					next = newNode()
					current.left = next
				}
				current = next
			}
		} // 木をコード長(huff.length)まで降りたところ
		// ここにテーブルのインデックスを入れる
		current.data = i
	}
	return root
}

/* テストデータ
[141,       127,       91       ]
[1000 1101, 0111 1111, 0101 1011]
   e   d    c  h   h    f   d  d

0 a 000,   4 e 100
1 b 001,   5 f 101
2 c 010,   6 g 110
3 d 011,   7 h 111
*/

// バイト配列を渡すと木を辿り
// その葉にあるテーブルへのインデックスの配列を返す。
func Decode(codes []byte) []byte {
	root := HuffmanTree
	// 空のバイト列はそのまま返す
	if len(codes) == 0 {
		return codes
	}

	// 初期化
	var result []byte
	var mask byte

	// スタート地点
	current := root
	for _, code := range codes { // バイト配列の頭から順に
		mask = 128 // 1000 0000 をマスクに使う。 1<<7 から定数に変更した
		// log.Printf("%b", code)
		for mask > 0 { // 8bit 読み終わるまで
			// log.Printf("%b %b %b", code, mask, code&mask)
			if code&mask == mask { // 1100 & 1000 = 1000 マスクすると先頭ビットが分かる
				current = current.right // 1 なら右
			} else {
				current = current.left // 0 なら左
			}
			if current.data != -1 { // 移動先で値を見つけたら
				// log.Println("==", current.data)

				// それを結果に追加、ここでは 256 までなので byte に変換
				result = append(result, byte(current.data))
				current = root // 残った bit を使ってまた根からたどる
			}
			mask = mask >> 1 // 1bit 処理したら減らす
		}
	}
	// 葉の値を見つけずに終わったら、それは EOS をたどる途中なので
	// 今は無視している。
	return result
}
