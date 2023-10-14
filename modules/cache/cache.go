package cache

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"fast-https/utils/files"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	rbt "github.com/emirpasic/gods/trees/redblacktree"
)

type CacheContainer struct {
	RbRoot *rbt.Tree
}

type CacheNode struct {
	Expire int
	Path   string
	Md5    string
}

type CacheHead CacheNode

type CacheEntry struct {
	Head CacheHead
	Size int64
	Data []byte
}

func init() {
	fmt.Println("-----[Fast-Https]cache init...")
}

// get all file-names in specific dir
func GetDirFiles(path string) ([]string, error) {
	var files []string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println("Error accessing file or directory:", err)
			return nil
		}
		if !info.IsDir() {
			files = append(files, filePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func Get_data_from_cache(realPath string) []byte {

	// return myMap.get(realPath)
	// if data == nil {
	// read_from_disk()
	// }
	// myMap.put(xx, xx)
	data, err := files.ReadFile(realPath)
	if err != nil {
		return nil
	}

	return data
}

func GetMd5(str string) string {
	data := []byte(str)
	md5New := md5.New()
	md5New.Write(data)
	// hex转字符串
	md5String := hex.EncodeToString(md5New.Sum(nil))
	// fmt.Println(md5String)
	// e10adc3949ba59abbe56e057f20f883e
	return md5String
}

// create a new cache container
func NewCache() *CacheContainer {
	return &CacheContainer{
		RbRoot: NewRBtree(),
	}
}

func RemoveFromDisk(node CacheNode) {
	// to do
}

func WriteToDisk(entry *CacheEntry) {
	name := entry.Head.Path + entry.Head.Md5 + ".gob"
	File, _ := os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0777)
	defer File.Close()

	enc := gob.NewEncoder(File)
	if err := enc.Encode(entry); err != nil {
		fmt.Println(err)
	}
}

func (CC *CacheContainer) ReadCache(str string, expire int, path string, data []byte, size int64) {
	md5 = GetMd5()
}

func (CC *CacheContainer) WriteCache(str string, expire int, path string, data []byte, size int64) {
	curr_time := int(time.Now().Unix())
	// Create a mew cache node
	var cacheNode CacheNode
	cacheNode.Expire = curr_time + expire
	cacheNode.Md5 = GetMd5(str)
	cacheNode.Path = path

	// put it in Rbtree
	// var node = &RBNode{
	// 	key:         Type(curr_time),
	// 	RbCacheNode: &cacheNode,
	// }
	AddInRbtree(CC.RbRoot, &cacheNode)

	var entry CacheEntry
	entry.Data = data
	entry.Head = CacheHead(cacheNode)
	entry.Size = size

	WriteToDisk(&entry) // async
}

func (CC *CacheContainer) LoadCache(dirPath string) {
	pathDec, _ := GetDirFiles(dirPath)

	var tmpentry CacheEntry
	for _, realPath := range pathDec {

	}
}
