package main

/*  ============ usage ============= */
/*  go run https.go http/https */
/*  http://localhost:1234/   or  https://localhost:1234/  */

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go [http|https]")
		return
	}

	protocol := os.Args[1]
	var url string
	var client *http.Client

	if protocol == "https" {
		url = "https://localhost:4443/"
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	} else if protocol == "http" {
		url = "http://localhost:4443/"
		client = &http.Client{}
	} else {
		fmt.Println("Invalid protocol. Use 'http' or 'https'.")
		return
	}

	// 发起GET请求
	resp, err := client.Get(url)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	fmt.Println("Response:", string(body))
}
