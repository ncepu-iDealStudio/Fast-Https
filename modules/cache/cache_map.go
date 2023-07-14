package cache

import (
	"fmt"
	"time"
)

const BucketsCount = 20

// node节点
type Node struct {
	Next *Node
	Data Value
}

// node节点存放的实际对象
type Value struct {
	Key   string
	Value []byte
	time  int64
}

// hashMap 桶
type HashMap struct {
	Buckets [BucketsCount]*Node //存在node节点的数组
}

// 新建一个hashMap桶
func NewHashMap() HashMap {
	hashMap := HashMap{}
	for i := 0; i < BucketsCount; i++ {
		hashMap.Buckets[i] = NewEmptyNode()
	}

	return hashMap
}

func Find() {

}

// 自定义hash算法获取key
func getBucketKey(key string) int {
	length := len(key)
	sum := 0
	for i := 0; i < length; i++ {
		sum = sum + int(key[i])
	}
	return sum % BucketsCount
}

// 在hashMap桶中新加一个节点
func (h *HashMap) put(data Value) {
	//获取index
	index := getBucketKey(data.Key)
	node := h.Buckets[index]
	//判断数组节点是否是空节点
	if node.Data.Value == nil {
		node.Data = data
	} else {
		//发生了hash碰撞,往该槽的链表尾巴处添加存放该数据对象的新节点
		last := node
		for last.Next != nil {
			last = last.Next
		}
		newNode := &Node{Data: data, Next: nil}
		last.Next = newNode
	}
}

// 从hashMap中获取某个key的值
func (h *HashMap) get(key string) []byte {
	//获取index
	index := getBucketKey(key)
	if h.Buckets[index].Data.Key == key {
		return h.Buckets[index].Data.Value
	}
	if h.Buckets[index].Next == nil {
		return nil
	}
	next := h.Buckets[index].Next
	for {
		if next.Data.Key == key {
			return next.Data.Value
		}
		if next.Next == nil {
			return nil
		}
		next = next.Next
	}
}

// 创建一个空node
func NewEmptyNode() *Node {
	node := &Node{}
	node.Data.Key = ""
	node.Data.Value = nil
	node.Next = nil
	node.Data.time = time.Now().Unix()
	return node
}

func MapTest() {
	myMap := NewHashMap()
	str := "this is string"
	data1 := Value{"001", []byte(str), 1}
	myMap.put(data1)
	data2 := Value{"002", []byte(str), 1}
	myMap.put(data2)
	fmt.Println(myMap.get("002"))
}
