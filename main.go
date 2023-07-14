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

	service.TestService("0.0.0.0:5000")
	server.Run()
	// select {}
}
