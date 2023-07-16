package main

import (
	_ "fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/server"
	"fast-https/service"
)

func main() {
	cache.LoadAllStatic()
	// cmd.Execute()

	service.TestService("127.0.0.1:5000")
	server.Run()

	// cache.TestCsGzip()
}
