package main

// Manual client for a server listening on localhost:4443.
// Usage (from the repository root): go run ./test/single/httpclient http|https

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./test/single/httpclient [http|https]")
		return
	}

	protocol := os.Args[1]
	var url string
	var client *http.Client

	switch protocol {
	case "https":
		url = "https://localhost:4443/"
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	case "http":
		url = "http://localhost:4443/"
		client = &http.Client{}
	default:
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	fmt.Println("Response:", string(body))
}
