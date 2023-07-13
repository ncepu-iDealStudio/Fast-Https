package main

import (
	_ "fast-https/config"
	"fast-https/modules/cache"
	"fast-https/modules/core/server"
)

func main() {

	cache.LoadAllStatic()

	server.Run()

}
