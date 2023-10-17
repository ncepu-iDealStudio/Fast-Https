package cache

import (
	"fast-https/modules/cache"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func WriteCache(str string, expire int, path string, data []byte, size int64) {
	curr_time := int(time.Now().Unix())
	// Create a mew cache node
	var cacheNode cache.CacheNode
	cacheNode.Expire = curr_time + expire
	cacheNode.Md5 = cache.GetMd5(str)
	cacheNode.Path = path

	var entry cache.CacheEntry
	entry.Data = data
	entry.Head = cache.CacheHead(cacheNode)
	entry.Size = size

	//WriteToDisk(&entry) // async
	//	将消息放入管道
	cache.CacheChan <- entry
}

func TestWriteCache(t *testing.T) {
	//	定义并发goroutine数量
	numGoroutines := 500
	// 定义要测试的文件的数量
	numFiles := numGoroutines

	//	创建一个等待组，用于等待所有并发goroutine完成
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 执行WriteToDisk方式
	go cache.WriteToDisk()

	//	创建一个目录用于文件写入测试
	testDir := "test_files"
	saveDir := "save_dir"
	os.Mkdir(saveDir, 0755)
	os.Mkdir(testDir, 0755)
	defer os.RemoveAll(testDir)

	// 生成500个文件的随机内容
	fileSizes := generateRandomFileSizes(numFiles)

	for i := 0; i < numGoroutines; i++ {
		i := i
		go func(size int) {
			defer wg.Done()

			// 生成随机文件内容
			fileContent := generateRandomFileContent(size)

			// 写入文件
			fileName, err := WriteFile(testDir, i, fileContent)
			//fileName := saveDir + "/file_" + strconv.Itoa(i)
			WriteCache(fileName+"test_dir", 10, "./"+saveDir+"/", fileContent, 10)
			if err != nil {
				t.Errorf("File write failed: %v", err)
			}
		}(fileSizes[i])
	}
	wg.Wait()
}

func generateRandomFileSizes(numFiles int) []int {
	sizes := make([]int, numFiles)
	for i := 0; i < numFiles; i++ {
		sizes[i] = rand.Intn(1024*1024) + 10240 // 10KB to 1MB
	}
	return sizes
}

func generateRandomFileContent(size int) []byte {
	content := make([]byte, size)
	rand.Read(content)
	return content
}

func WriteFile(dir string, index int, content []byte) (string, error) {
	fileName := dir + "/file_" + strconv.Itoa(index)
	return fileName, ioutil.WriteFile(fileName, content, 0644)
}
