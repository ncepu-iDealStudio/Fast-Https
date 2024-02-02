/**
* @Author: Ajax, yizhigopher, 彭博
* @Description: cache server and manage
* @File: cache_validate.go
* @Version: 1.0.0
* @Date: 2023/10/19 21:38:41
 */

package cache

import (
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"fast-https/config"
	"fast-https/utils/files"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type CacheContainer struct {
	RbRoot *RBtree
}

type CacheNode struct {
	Expire int
	Valid  int
	Path   string
	Md5    string
}

type CacheHead CacheNode

type CacheEntry struct {
	Head CacheHead
	Size int
	Data []byte
}

var CacheChan chan CacheEntry
var GCacheContainer *CacheContainer

func init() {
	// fmt.Println("-----[Fast-Https]cache init...")
	CacheChan = make(chan CacheEntry, 1000)
	go WriteToDisk()
	GCacheContainer = NewCacheContainer()
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
func NewCacheContainer() *CacheContainer {
	return &CacheContainer{
		RbRoot: NewRBtree(),
	}
}

func RemoveFromDisk(node CacheNode) {
	realPath := node.Path
	realPath = filepath.Join(realPath, node.Md5)
	os.Remove(realPath)
}

func WriteToDisk() {
	// 不断的阻塞的等待channel的消息
	for {
		select {
		case entry := <-CacheChan:
			// name := entry.Head.Path + entry.Head.Md5 + ".gob"
			savePath := entry.Head.Path
			realPath := filepath.Join(savePath, entry.Head.Md5)
			if _, err := os.Stat(savePath); os.IsNotExist(err) {
				err := os.MkdirAll(savePath, 0755)
				if err != nil {
					fmt.Println("无法创建目录:", err)
					return
				}
				//fmt.Println("目录已创建:", savePath)
			}

			// fmt.Println("writing data to:", realPath)
			File, _ := os.OpenFile(realPath, os.O_RDWR|os.O_CREATE, 0777)
			// defer File.Close()
			enc := gob.NewEncoder(File)
			if err := enc.Encode(entry); err != nil {
				fmt.Println(err)
			}
			File.Close()

		}
	}
}

// this should run once, when server init
func (CC *CacheContainer) LoadCache() {
	for _, server := range config.GConfig.Servers {
		for _, path := range server.Path {
			if path.ProxyCache.Path != "" {
				files, _ := GetDirFiles(path.ProxyCache.Path)
				for _, file := range files {
					node := CacheNode(getCacheHead(file))
					CC.RbRoot.AddInRbtree(&node)
				}
			}
		}
	}
}

func (CC *CacheContainer) WriteCache(str string, expire int, path string, data []byte, size int) {
	curr_time := int(time.Now().Unix())
	// Create a mew cache node

	var cacheNode CacheNode
	cacheNode.Expire = curr_time + expire
	cacheNode.Md5 = str
	n1 := 1
	n2 := 2
	entryHeadMd5Len := len(cacheNode.Md5)
	savePath := filepath.Join(path, cacheNode.Md5[entryHeadMd5Len-n1:],
		cacheNode.Md5[entryHeadMd5Len-n1-n2:entryHeadMd5Len-n1])
	cacheNode.Path = savePath
	cacheNode.Valid = expire

	// put it in Rbtree
	// var node = &RBNode{
	// 	key:         Type(curr_time),
	// 	RbCacheNode: &cacheNode,
	// }
	// fmt.Println(cacheNode)
	CC.RbRoot.AddInRbtree(&cacheNode)

	var entry CacheEntry
	entry.Data = data
	entry.Head = CacheHead(cacheNode)
	entry.Size = size

	//WriteToDisk(&entry) // async
	CacheChan <- entry

}

func (CC *CacheContainer) ReadCache(strMd5 string) (data []byte, flag bool) {
	data = []byte("")

	cacheNode, flag := CC.RbRoot.GetNodeFromRBtreeByKey(strMd5)

	if flag {
		realPath := filepath.Join(cacheNode.Path, strMd5)
		file, err := os.OpenFile(realPath, os.O_RDWR, 0777)

		if err != nil {
			fmt.Println(err)
			return data, false
		}
		var entry CacheEntry

		enc := gob.NewDecoder(file)
		if err := enc.Decode(&entry); err != nil {
			fmt.Println("encode ", err)
		}
		file.Close()

		data = entry.Data
	}

	return
}

func getCacheHead(realPath string) (head CacheHead) {
	var entry CacheEntry

	file, err := os.OpenFile(realPath, os.O_RDWR, 0777)
	if err != nil {
		fmt.Println(err)
		return CacheHead{}
	}
	enc := gob.NewDecoder(file)
	if err := enc.Decode(&entry); err != nil {
		fmt.Println("encode ", err)
	}
	file.Close()

	head = entry.Head

	return
}
