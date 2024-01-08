package service

import (
	"fmt"
	"log"
	"net/http"
)

var testport1 string

func HttpHandle(rw http.ResponseWriter, req *http.Request) {
	// req.ParseForm() //解析参数，默认是不会解析的
	// for key, val := range req.Form {
	// 	fmt.Println("-------------------------")
	// 	fmt.Println("key:", key)
	// 	fmt.Println("val:", strings.Join(val, ""))
	// }
	str := "<h1> This is a Simple test service on </h1>" + testport1 + "path: " + req.RequestURI
	fmt.Fprintf(rw, str)
}

func TestService(port1 string, str string) {
	testport1 = port1

	go func() {
		http.HandleFunc("/", HttpHandle)       //设置访问的路由
		err := http.ListenAndServe(port1, nil) //设置监听的端口
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()
	// fmt.Println("start test service")
}
