package cache

import (
	"fmt"
	"time"
)

const BucketsCount = 131

// node节点
type Node struct {
	Next *Node
	Data CacheNode
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
func (h *HashMap) AddMap(data CacheNode) {
	//获取index
	index := getBucketKey(data.Md5)
	node := h.Buckets[index]
	//判断数组节点是否是空节点
	if node.Data.Md5 == "" {
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
func (h *HashMap) FindInMap(key string) *CacheNode {
	//获取index
	index := getBucketKey(key)
	if h.Buckets[index].Data.Md5 == key {
		return &h.Buckets[index].Data
	}
	if h.Buckets[index].Next == nil {
		return nil
	}
	next := h.Buckets[index].Next
	for {
		if next.Data.Md5 == key {
			return &next.Data
		}
		if next.Next == nil {
			return nil
		}
		next = next.Next
	}
}

func (h *HashMap) DeleteInMap(key string) {
	// 获取index
	index := getBucketKey(key)
	if h.Buckets[index].Data.Md5 == key {
		h.Buckets[index] = nil // delete this node
	}

	next := h.Buckets[index].Next
	for {
		if next.Next.Next == nil {
			next.Next = nil
			break
		}
		next = next.Next
	}
}

// 创建一个空node
func NewEmptyNode() *Node {
	node := &Node{}
	node.Data.Md5 = ""
	node.Data.Path = ""
	node.Data.Expire = int(time.Now().Unix())
	node.Next = nil
	return node
}

func MapTest() {
	myMap := NewHashMap()
	data1 := CacheNode{15, "001", "this is 12"}
	myMap.AddMap(data1)
	data2 := CacheNode{12, "002", "this is 15"}
	myMap.AddMap(data2)
	fmt.Println(myMap.FindInMap("this is 15"))
}
