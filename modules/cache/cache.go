package cache

import (
	"crypto/md5"
	"encoding/hex"
	"fast-https/config"
	"fast-https/utils/files"
	"fast-https/utils/message"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var myMap = NewHashMap()

func init() {
	// fmt.Println("-----[Fast-Https]cache init...")
}

// get all file-names in specific dir
func SearchDirFiles(path string) ([]string, error) {
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

func LoadAllStatic() {
	message.PrintInfo("loadstatic")
	for _, server := range config.G_config.Servers {
		for _, path := range server.Path {

			if path.Root != "" {
				dir, err := SearchDirFiles(path.Root)
				if err != nil {
					message.PrintErr("search files in dir error")
				}

				for _, realPath := range dir {
					data, _ := files.ReadFile(realPath)
					flag := false
					if path.Zip == 1 {
						data, _ = CompressBytes_Gzip(data)
						flag = true
					}
					if config.G_OS == "windows" {
						realPath = "/" + realPath
						realPath = strings.ReplaceAll(realPath, "\\", "/")
					}
					myMap.put(Value{realPath, data, time.Now().Unix()})

					if flag {
						message.PrintInfo("Cached gzip ", realPath)
					} else {
						message.PrintInfo("Cached file ", realPath)
					}
				}
			}
		}

	}
}

func Get_data_from_cache(realPath string) []byte {

	return myMap.get(realPath)
	// if data == nil {
	// read_from_disk()
	// }
	// myMap.put(xx, xx)
}

func Release_cache() {
	time.Sleep(60 * time.Second)

}

func Test() {
	str := "123456"
	data := []byte(str)
	md5New := md5.New()
	md5New.Write(data)
	// hex转字符串
	md5String := hex.EncodeToString(md5New.Sum(nil))
	fmt.Println(md5String)
	// e10adc3949ba59abbe56e057f20f883e
}
